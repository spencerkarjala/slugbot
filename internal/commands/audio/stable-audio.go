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

	"github.com/bwmarrin/discordgo"
)

type StableAudioCommand struct {
	commands.Command
	traits.Promptable
}

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

func (cmd *StableAudioCommand) Apply() error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	triggeringMessage := &discordgo.MessageReference{
		MessageID: cmd.Message.ID,
		ChannelID: cmd.Message.ChannelID,
	}

	content := strings.TrimSpace(cmd.Message.Content)
	parts := strings.SplitN(content, " ", 2)
	if len(parts) < 2 {
		cmd.Session.ChannelMessageSendReply(cmd.Message.ChannelID, "Usage: .saudio <prompt>", triggeringMessage)
		return nil
	}
	prompt := parts[1:]

	initMsg, err := cmd.Session.ChannelMessageSendReply(
		cmd.Message.ChannelID,
		fmt.Sprintf("Generating audio for prompt: `%s`...", strings.Join(prompt, " ")),
		triggeringMessage,
	)
	if err != nil {
		return fmt.Errorf("failed to send initial message: %w", err)
	}

	// Prepare output filename
	timestamp := time.Now().Unix()
	outFile := fmt.Sprintf("saudio_%d.wav", timestamp)
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

	// Invoke the Stable Audio CLI via Conda inside a login shell
	// so that conda initialization is applied and the CLI command is found.
	shellCmd := fmt.Sprintf(
		"./stable-audio/sag --prompt %q --output %s --progress_file %s",
		strings.Join(prompt, " "), outFile, progressFile,
	)
	command := exec.Command("bash", "-lc", shellCmd)

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
