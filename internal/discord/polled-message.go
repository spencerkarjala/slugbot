package discord

import (
	"time"

	"slugbot/internal/io/slog"
	"slugbot/internal/utils"
)

// FilePollMessage ties a Discord message to a polled‐file.
type FilePollMessage struct {
	Message    *Message
	PolledFile *utils.PollableFile
	done       chan struct{}
	FilePath   string
}

// NewFilePollMessage constructs the object.  interval is your polling interval.
func NewFilePollMessage(api SessionAPI, channelID string, replyToMessageID string, interval time.Duration) (*FilePollMessage, error) {
	msg, err := NewReplyMessage(api, channelID, replyToMessageID)
	if err != nil {
		return nil, err
	}

	done := make(chan struct{})

	pf, err := utils.NewPollableFile(interval, func(text string) {
		err = msg.Update(text)
		if err != nil {
			slog.Error("Failed to update message: %w", err)
		}
	})
	if err != nil {
		return nil, err
	}

	return &FilePollMessage{
		Message:    msg,
		PolledFile: pf,
		done:       done,
		FilePath:   pf.File,
	}, nil
}

// Start sends the first message with initialText, then begins polling.
// After Start returns, an external process can write to fp.FilePath to drive updates to the message.
func (fpm *FilePollMessage) Start(initialText string) error {
	if err := fpm.Message.Create(initialText); err != nil {
		return err
	}
	go fpm.PolledFile.Start(fpm.done)
	return nil
}

// Stop halts polling and deletes the Discord message.
func (fpm *FilePollMessage) Stop() error {
	close(fpm.done)
	return fpm.Message.Delete()
}
