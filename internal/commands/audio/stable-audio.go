package audio

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"slugbot/internal/commands"
	"slugbot/internal/commands/traits"
	"slugbot/internal/io/slog"

	"github.com/bwmarrin/discordgo"
)

type StableAudioCommand struct {
	commands.Command
	traits.Promptable
}

type StableAudioParams struct {
	Length         float64
	Strength       float64
	Prompt         string
	NegativePrompt string
}

var re = regexp.MustCompile(`\s+`)

// SetContext captures Discord context and extracts the prompt text.
func (c *StableAudioCommand) SetContext(s *discordgo.Session, m *discordgo.MessageCreate) {
	c.Command.SetContext(s, m)
	content := strings.TrimSpace(m.Content)
	parts := strings.Split(content, " ")
	if len(parts) > 1 {
		c.Promptable.SetPrompt(strings.Join(parts[1:], " "))
	} else {
		c.Promptable.SetPrompt("")
	}
}

func (c *StableAudioCommand) Usage() string {
	return "Usage: `.saudio <prompt>`"
}

func (c *StableAudioCommand) Validate() error {
	if c.Session == nil {
		return fmt.Errorf("invalid session reference")
	}
	if c.Message == nil {
		return fmt.Errorf("invalid message reference")
	}

	args := strings.Fields(c.Message.Content)

	if len(args) < 2 {
		return errors.New(c.Usage())
	}

	return nil
}

func ParseArgs(args []string) (*StableAudioParams, error) {
	params := &StableAudioParams{
		Length:         30.0,
		Strength:       7.0,
		Prompt:         "",
		NegativePrompt: "",
	}

	// parse params; TODO: make this more general/abstracted
	i := 0
	prompt := []string{}
	negativePrompt := []string{}
	collectNegative := false
	for i < len(args) {
		switch args[i] {
		case "--length":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for --length")
			}
			length, err := strconv.ParseFloat(args[i+1], 64)
			if err != nil || length <= 0.0 {
				return nil, fmt.Errorf("invalid length: %v", args[i+1])
			}
			params.Length = length
			i += 2

		case "--strength":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for --strength")
			}
			strength, err := strconv.ParseFloat(args[i+1], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid strength: %v", args[i+1])
			}
			params.Strength = strength
			i += 2

		case "--negative":
			collectNegative = true
			i++

		default:
			if !collectNegative {
				prompt = append(prompt, args[i])
			} else {
				negativePrompt = append(negativePrompt, args[i])
			}
			i++
		}
	}

	params.Prompt = strings.Join(prompt, " ")
	params.NegativePrompt = strings.Join(negativePrompt, " ")

	slog.Info("Got prompt:          ", params.Prompt)
	slog.Info("Got negative prompt: ", params.NegativePrompt)
	slog.Info("    strength:        ", params.Strength)
	slog.Info("    length:          ", params.Length)

	if params.Prompt == "" {
		return nil, fmt.Errorf("prompt is empty")
	}

	return params, nil
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) > max {
		return string(r[:max])
	}
	return s
}

func makeFilename(params *StableAudioParams, timestamp int64) string {
	combinedStr := ""
	if params.Prompt != "" {
		combinedStr += truncate(params.Prompt, 100)
	}
	if params.Prompt != "" && params.NegativePrompt != "" {
		combinedStr += "-"
	}
	if params.NegativePrompt != "" {
		combinedStr += truncate(params.NegativePrompt, 100)
	}
	baseString := re.ReplaceAllString(combinedStr, "-")

	return fmt.Sprintf("saudio-%s-%d.wav", baseString, timestamp)
}

func (cmd *StableAudioCommand) Apply() error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	triggeringMessage := &discordgo.MessageReference{
		MessageID: cmd.Message.ID,
		ChannelID: cmd.Message.ChannelID,
	}

	content := strings.TrimSpace(cmd.Message.Content)
	parts := strings.Split(content, " ")
	if len(parts) < 2 {
		cmd.Session.ChannelMessageSendReply(cmd.Message.ChannelID, "Usage: .saudio <prompt>", triggeringMessage)
		return nil
	}
	params, err := ParseArgs(parts[1:])
	if err != nil {
		slog.Error("failed to parse args: %v", err)
		return err
	}

	initMsg, err := cmd.Session.ChannelMessageSendReply(
		cmd.Message.ChannelID,
		fmt.Sprintf("Generating audio for prompt: `%s`...\r\nnegative prompt: `%s`", params.Prompt, params.NegativePrompt),
		triggeringMessage,
	)
	if err != nil {
		return fmt.Errorf("failed to send initial message: %w", err)
	}

	timestamp := time.Now().Unix()
	outFile := makeFilename(params, timestamp)
	progressFile := fmt.Sprintf("saudio_%d.progress", timestamp)

	// Start background goroutine to poll progress and edit message
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				cmd.Session.ChannelMessageDelete(initMsg.ChannelID, initMsg.ID)
				return
			case <-ticker.C:
				data, err := os.ReadFile(progressFile)
				if err != nil {
					continue
				}
				text := strings.TrimSpace(string(data))
				if text != "" {
					cmd.Session.ChannelMessageEdit(initMsg.ChannelID, initMsg.ID,
						fmt.Sprintf("`%s`", text),
					)
				}
			}
		}
	}()

	// 1. Check for an uploaded WAV attachment
	var initAudioPath string
	for _, att := range cmd.Message.Attachments {
		if strings.HasSuffix(att.Filename, ".wav") {
			slog.Trace("Trying to download audio from: ", att.URL)

			resp, err := http.Get(att.URL)
			if err != nil {
				slog.Error("failed to download init audio:", err)
				return fmt.Errorf("failed to download audio input")
			}
			defer resp.Body.Close()

			slog.Trace("Got response: ", resp)

			tmpf, err := os.CreateTemp("", "saudio-init-*.wav")
			if err != nil {
				slog.Error("failed to create temp file:", err)
				return fmt.Errorf("failed to download audio input")
			}
			defer tmpf.Close()

			slog.Trace("Created temporary file for input: ", tmpf.Name())

			if _, err := io.Copy(tmpf, resp.Body); err != nil {
				slog.Error("failed to save init audio:", err)
				return fmt.Errorf("failed to download audio input")
			}
			initAudioPath = tmpf.Name()

			slog.Trace("Downloaded data into file: ", initAudioPath)
			break
		}
	}

	cmdArgs := []string{
		"--prompt", params.Prompt,
		"--negative_prompt", params.NegativePrompt,
		"--output", outFile,
		"--progress_file", progressFile,
		"--cfg_scale", fmt.Sprintf("%0.2f", params.Strength),
		"--length", fmt.Sprintf("%0.2f", params.Length),
	}
	if initAudioPath != "" {
		slog.Info("Using input audio file: ", initAudioPath)
		cmdArgs = append(cmdArgs, "--init_audio", initAudioPath)
	} else {
		slog.Info("No input audio detected; proceeding with text only")
	}
	command := exec.Command("./stable-audio/sag", cmdArgs...)

	// Run the command and capture any errors or output
	if output, err := command.CombinedOutput(); err != nil {
		errMsg := fmt.Sprintf("Error during audio generation: %v\n%s", err, string(output))
		cmd.Session.ChannelMessageEdit(cmd.Message.ChannelID, initMsg.ID, errMsg)
		return err
	}
	close(done)

	// Send the resulting audio file back to the Discord channel
	file, err := os.Open(outFile)
	if err != nil {
		cmd.Session.ChannelMessageSendReply(cmd.Message.ChannelID, "Failed to open output file: "+err.Error(), triggeringMessage)
		return err
	}
	defer file.Close()

	finalMessage := &discordgo.MessageSend{
		Files: []*discordgo.File{{
			Name:   outFile,
			Reader: file,
		}},
		Reference: triggeringMessage,
	}

	if _, err := cmd.Session.ChannelMessageSendComplex(cmd.Message.ChannelID, finalMessage); err != nil {
		cmd.Session.ChannelMessageSend(cmd.Message.ChannelID, "Failed to send file: "+err.Error())
		return err
	}

	return nil
}
