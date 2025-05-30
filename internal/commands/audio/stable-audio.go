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
	"slugbot/internal/discord"
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
	Seed           int64
	Steps          int64
	IsSmall        bool
}

var whitespaceRegex = regexp.MustCompile(`\s+`)
var forwardSlashRegex = regexp.MustCompile(`/`)

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
		Seed:           -1,
		Steps:          100,
		IsSmall:        false,
	}

	// parse params; TODO: make this more general/abstracted
	i := 0
	prompt := []string{}
	negativePrompt := []string{}
	collectNegative := false
	stepsSet := false
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

		case "--seed":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for --seed")
			}
			seed, err := strconv.ParseInt(args[i+1], 10, 64)
			if err != nil || seed < 0 {
				return nil, fmt.Errorf("invalid seed '%q' (needs to be a positive integer): %w", params.Seed, err)
			}
			params.Seed = seed
			i += 2

		case "--steps":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for --steps")
			}
			steps, err := strconv.ParseInt(args[i+1], 10, 64)
			if err != nil || steps < 0 {
				return nil, fmt.Errorf("invalid steps '%q' (needs to be >0): %w", params.Steps, err)
			}
			params.Steps = steps
			i += 2
			stepsSet = true

		case "--negative":
			collectNegative = true
			i++

		case "--small":
			params.IsSmall = true
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

	if !stepsSet && params.IsSmall {
		params.Steps = 8
	}

	params.Prompt = strings.Join(prompt, " ")
	params.NegativePrompt = strings.Join(negativePrompt, " ")

	slog.Info("Got prompt:          ", params.Prompt)
	slog.Info("Got negative prompt: ", params.NegativePrompt)
	slog.Info("    strength:        ", params.Strength)
	slog.Info("    length:          ", params.Length)
	slog.Info("    seed:            ", params.Seed)
	slog.Info("    steps:           ", params.Steps)
	slog.Info("    small?           ", params.IsSmall)

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
	baseString := whitespaceRegex.ReplaceAllString(combinedStr, "-")
	baseString = forwardSlashRegex.ReplaceAllString(baseString, "")

	return fmt.Sprintf("saudio-%s-%d.wav", baseString, timestamp)
}

func downloadAndSave(url string) (string, error) {
	slog.Trace("Trying to download audio from: ", url)

	resp, err := http.Get(url)
	if err != nil {
		slog.Error("failed to download init audio:", err)
		return "", fmt.Errorf("failed to download audio input")
	}
	defer resp.Body.Close()

	tmpf, err := os.CreateTemp("", "saudio-init-*.wav")
	if err != nil {
		slog.Error("failed to create temp file:", err)
		return "", fmt.Errorf("failed to download audio input")
	}
	defer tmpf.Close()

	slog.Trace("Created temporary file for input: ", tmpf.Name())

	if _, err := io.Copy(tmpf, resp.Body); err != nil {
		slog.Error("failed to save init audio:", err)
		return "", fmt.Errorf("failed to download audio input")
	}
	return tmpf.Name(), nil
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
	if string(parts[0]) == ".saudiosm" {
		parts = append(parts, "--small")
	}
	if len(parts) < 2 {
		cmd.Session.ChannelMessageSendReply(cmd.Message.ChannelID, "Usage: .saudio <prompt>", triggeringMessage)
		return nil
	}
	params, err := ParseArgs(parts[1:])
	if err != nil {
		slog.Error("failed to parse args: %v", err)
		return err
	}

	timestamp := time.Now().Unix()
	outFile := makeFilename(params, timestamp)

	fp, err := discord.NewFilePollMessage(
		discord.ConcreteSession{Session: cmd.Session},
		cmd.Message.ChannelID,
		triggeringMessage.MessageID,
		1*time.Second,
	)
	if err != nil {
		return fmt.Errorf("failed to init progress poller: %w", err)
	}

	initMsgString := fmt.Sprintf("Generating audio for prompt: `%s`...\r\nnegative prompt: `%s`", params.Prompt, params.NegativePrompt)
	if err := fp.Start(initMsgString); err != nil {
		return fmt.Errorf("failed to start progress poller: %w", err)
	}
	defer fp.Stop()

	progressFile := fp.FilePath

	// if an uploaded wav is attached, use it as the input audio
	var initAudioPath string
	for _, att := range cmd.Message.Attachments {
		if strings.HasSuffix(att.Filename, ".wav") {
			initAudioPath, err = downloadAndSave(att.URL)
			if err != nil {
				slog.Error("failed to download init audio: %v", err)
				return fmt.Errorf("failed to download audio input")
			}

			slog.Trace("Downloaded data into file: ", initAudioPath)
			break
		}
	}

	// if no input audio so far, and the message replies to a message with attached
	// audio, then use that as the input audio
	if initAudioPath == "" && cmd.Message.MessageReference != nil {
		refMsg, err := cmd.Session.ChannelMessage(
			cmd.Message.ChannelID,
			cmd.Message.MessageReference.MessageID,
		)
		if err != nil {
			slog.Warn("could not fetch referenced message: ", err)
		} else {
			for _, att := range refMsg.Attachments {
				if strings.HasSuffix(att.Filename, ".wav") {
					initAudioPath, err = downloadAndSave(att.URL)
					if err != nil {
						slog.Error("failed to download init audio: %v", err)
						return fmt.Errorf("failed to download audio input from reply")
					}
					break
				}
			}
		}
	}

	cmdArgs := []string{
		"--prompt", params.Prompt,
		"--negative_prompt", params.NegativePrompt,
		"--output", outFile,
		"--progress_file", progressFile,
		"--cfg_scale", fmt.Sprintf("%0.2f", params.Strength),
		"--length", fmt.Sprintf("%0.2f", params.Length),
		"--seed", fmt.Sprintf("%d", params.Seed),
		"--steps", fmt.Sprintf("%d", params.Steps),
	}
	if initAudioPath != "" {
		slog.Info("Using input audio file: ", initAudioPath)
		cmdArgs = append(cmdArgs, "--init_audio", initAudioPath)
	} else {
		slog.Info("No input audio detected; proceeding with text only")
	}
	if params.IsSmall {
		slog.Info("Using small model")
		cmdArgs = append(cmdArgs, "--small")
	}
	command := exec.Command("./stable-audio/sag", cmdArgs...)

	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	// Run the command
	if err := command.Run(); err != nil {
		err = fmt.Errorf("error during audio generation: %w", err)
		if stopErr := fp.Stop(); stopErr != nil {
			err = fmt.Errorf("%w; during handling, another error occurred: %w", err, stopErr)
		}
		slog.Error(err.Error())

		errorMessage, createMessageErr := discord.NewMessage(discord.ConcreteSession{Session: cmd.Session}, cmd.Message.ChannelID)
		if createMessageErr != nil {
			err = fmt.Errorf("%w; when creating a new discord message, another error occurred: %w", err, createMessageErr)
			return err
		}

		if sendMessageErr := errorMessage.Create(err.Error()); sendMessageErr != nil {
			err = fmt.Errorf("%w; when sending the error message to discord, another error occurred: %w", err, sendMessageErr)
		}

		return err
	}

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
