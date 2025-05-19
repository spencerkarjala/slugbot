// stable-audio-with-config.go
package audio

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"slugbot/internal/commands"
	"slugbot/internal/commands/traits"
	"slugbot/internal/discord"
	"slugbot/internal/io/slog"

	"github.com/BurntSushi/toml"
	"github.com/bwmarrin/discordgo"
)

type StableAudioWithConfigCommand struct {
	commands.Command
	traits.Promptable
}

type StableAudioWithConfigParams struct {
	Prompts         map[string]float64 `toml:"prompts"`
	NegativePrompts map[string]float64 `toml:"neg_prompts"`
}

func (c *StableAudioWithConfigCommand) makeFilename(params *StableAudioWithConfigParams, timestamp int64) string {
	combinedStr := ""
	for prompt, weight := range params.Prompts {
		combinedStr += fmt.Sprintf("%s %0.2f ", prompt, weight)
	}
	if len(params.Prompts) > 0 && len(params.NegativePrompts) > 0 {
		combinedStr += "-"
	}
	for prompt, weight := range params.NegativePrompts {
		combinedStr += fmt.Sprintf("%s %0.2f", prompt, weight)
	}
	baseString := whitespaceRegex.ReplaceAllString(combinedStr, "-")
	baseString = forwardSlashRegex.ReplaceAllString(baseString, "")

	return fmt.Sprintf("saudio-%s-%d.wav", baseString, timestamp)
}

func (c *StableAudioWithConfigCommand) SetContext(s *discordgo.Session, m *discordgo.MessageCreate) {
	c.Command.SetContext(s, m)
}

func (c *StableAudioWithConfigCommand) Usage() string {
	return "Usage: ```saudio\n<toml config>\n```"
}

func (c *StableAudioWithConfigCommand) Validate() error {
	if c.Session == nil || c.Message == nil {
		return errors.New("invalid context")
	}
	content := c.Message.Content
	len_content := len(content)
	if len_content < 13 {
		slog.Warn("StableAudioWithConfig.Validate: got short message")
		return errors.New(c.Usage())
	}
	if content[:9] != "```saudio" {
		err := fmt.Errorf("StableAudioWithConfig.Validate: got invalid message start: %s", content[0:9])
		slog.Warn(err.Error())
		return err
	}
	if content[len_content-3:len_content] != "```" {
		err := fmt.Errorf("StableAudioWithConfig.Validate: got invalid message end: %s", content[len_content-3:len_content])
		slog.Warn(err.Error())
		return err
	}
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "```saudio") || !strings.HasSuffix(content, "```") {
		return errors.New(c.Usage())
	}
	return nil
}

func ParseTOML(content string) (*StableAudioWithConfigParams, error) {
	params := StableAudioWithConfigParams{
		Prompts:         map[string]float64{},
		NegativePrompts: map[string]float64{},
	}
	if _, err := toml.Decode(content, &params); err != nil {
		return nil, err
	}
	return &params, nil
}

func (cmd *StableAudioWithConfigCommand) Apply() error {
	if err := cmd.Validate(); err != nil {
		return err
	}

	content := cmd.Message.Content[9 : len(cmd.Message.Content)-3]
	params, err := ParseTOML(content)
	if err != nil {
		return fmt.Errorf("failed to parse toml: %w", err)
	}

	triggeringMessage := &discordgo.MessageReference{
		MessageID: cmd.Message.ID,
		ChannelID: cmd.Message.ChannelID,
	}

	parts := strings.SplitN(strings.TrimSpace(cmd.Message.Content), "\n", 2)
	if len(parts) < 2 {
		return errors.New("invalid saudio block")
	}
	toml := strings.TrimSuffix(parts[1], "```")

	fp, err := discord.NewFilePollMessage(
		discord.ConcreteSession{Session: cmd.Session},
		triggeringMessage.ChannelID,
		triggeringMessage.MessageID,
		1*time.Second,
	)
	if err != nil {
		return fmt.Errorf("failed to init progress poller: %w", err)
	}

	timestamp := time.Now().Unix()
	outFile := cmd.makeFilename(params, timestamp)

	initMsgString := fmt.Sprintf("Generating audio for file %s...", outFile)
	slog.Info(initMsgString)
	if err := fp.Start(initMsgString); err != nil {
		return fmt.Errorf("failed to start progress poller: %w", err)
	}
	defer fp.Stop()

	progressFile := fp.FilePath
	slog.Info("Using progressFile: ", fp.FilePath)

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
		"--toml",
		"--progress_file", progressFile,
		"--output", outFile,
	}
	if initAudioPath != "" {
		slog.Info("Using input audio file: ", initAudioPath)
		cmdArgs = append(cmdArgs, "--init_audio", initAudioPath)
	} else {
		slog.Info("No input audio detected; proceeding with text only")
	}

	// 4) Invoke sag, piping TOML to stdin
	command := exec.Command("./stable-audio/sag", cmdArgs...)
	command.Stdin = strings.NewReader(toml)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

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
