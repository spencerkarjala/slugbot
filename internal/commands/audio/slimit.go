package audio

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"slugbot/internal/commands"
	"slugbot/internal/io/slog"

	"github.com/bwmarrin/discordgo"
)

// LimitCommand applies the Python limiter to a WAV and re-uploads it.
type LimitCommand struct {
	commands.Command
}

// Usage shows basic help for .slimit
func (c *LimitCommand) Usage() string {
	return "Usage: `.slimit` (reply to or attach a .wav file)"
}

func (c *LimitCommand) Validate() error {
	if c.Session == nil || c.Message == nil {
		return fmt.Errorf("invalid session or message")
	}
	// no extra args expected
	return nil
}

func (c *LimitCommand) Apply() error {
	if err := c.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	triggering := &discordgo.MessageReference{
		MessageID: c.Message.ID,
		ChannelID: c.Message.ChannelID,
	}

	// 1) find source WAV URL
	var srcURL string
	for _, att := range c.Message.Attachments {
		if strings.HasSuffix(att.Filename, ".wav") {
			srcURL = att.URL
			break
		}
	}
	if srcURL == "" && c.Message.MessageReference != nil {
		ref, err := c.Session.ChannelMessage(
			c.Message.ChannelID,
			c.Message.MessageReference.MessageID,
		)
		if err == nil {
			for _, att := range ref.Attachments {
				if strings.HasSuffix(att.Filename, ".wav") {
					srcURL = att.URL
					break
				}
			}
		}
	}
	if srcURL == "" {
		c.Session.ChannelMessageSendReply(c.Message.ChannelID,
			"No WAV found to limit", triggering)
		return nil
	}

	// 2) download to temp file
	tmpIn, err := downloadAndSave(srcURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpIn)

	// 3) run limiter script
	py_path := filepath.Join(".conda", "general-dsp", "bin", "python")
	outFile := fmt.Sprintf("slimit-%d.wav", time.Now().Unix())
	cmd := exec.Command(
		py_path, "py/limiter.py",
		"--input", tmpIn,
		"--output", outFile,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("limiter failed: %w", err)
	}
	defer os.Remove(outFile)

	// 4) open & send
	f, err := os.Open(outFile)
	if err != nil {
		return fmt.Errorf("open output: %w", err)
	}
	defer f.Close()

	msg := &discordgo.MessageSend{
		Files: []*discordgo.File{{
			Name:   outFile,
			Reader: f,
		}},
		Reference: triggering,
	}
	if _, err := c.Session.ChannelMessageSendComplex(
		c.Message.ChannelID, msg,
	); err != nil {
		return fmt.Errorf("send failed: %w", err)
	}

	slog.Info("Delivered limited file:", outFile)
	return nil
}
