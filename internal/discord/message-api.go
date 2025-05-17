package discord

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/bwmarrin/discordgo"
)

// ErrUnknownMessage is returned when Discord reports an unknown message (404 or UnknownMessage code).
var ErrUnknownMessage = errors.New("discord: unknown message")

// ConcreteSession wraps a discordgo.Session and implements SessionAPI.
type ConcreteSession struct {
	Session *discordgo.Session
}

type ConcreteMessage struct {
	ID string
}

// Check returns an error if the underlying session is invalid; nil otherwise.
func (api ConcreteSession) Check() error {
	if api.Session == nil {
		return fmt.Errorf("Check: invalid session reference")
	}
	return nil
}

// ChannelMessage fetches a single message. It wraps Discord REST errors into ErrUnknownMessage when appropriate.
func (api ConcreteSession) ChannelMessage(channelID, messageID string) (ConcreteMessage, error) {
	msg, err := api.Session.ChannelMessage(channelID, messageID)
	if err != nil {
		if restErr, ok := err.(*discordgo.RESTError); ok {
			if (restErr.Message != nil && restErr.Message.Code == discordgo.ErrCodeUnknownMessage) ||
				(restErr.Response != nil && restErr.Response.StatusCode == http.StatusNotFound) {
				return ConcreteMessage{}, ErrUnknownMessage
			}
		}
		return ConcreteMessage{}, err
	}
	return ConcreteMessage{ID: msg.ID}, nil
}

// ChannelMessageSend sends a new message to the channel. Errors are passed through directly.
func (api ConcreteSession) ChannelMessageSend(channelID, content string) (ConcreteMessage, error) {
	msg, err := api.Session.ChannelMessageSend(channelID, content)
	if err != nil {
		return ConcreteMessage{}, err
	}
	return ConcreteMessage{ID: msg.ID}, nil
}

// ChannelMessageEdit edits an existing message’s content. Errors are passed through directly.
func (api ConcreteSession) ChannelMessageEdit(channelID, messageID, content string) error {
	_, err := api.Session.ChannelMessageEdit(channelID, messageID, content)
	return err
}

// ChannelMessageDelete deletes the specified message. It wraps Discord REST errors into ErrUnknownMessage when appropriate.
func (api ConcreteSession) ChannelMessageDelete(channelID, messageID string) error {
	err := api.Session.ChannelMessageDelete(channelID, messageID)
	if err != nil {
		if restErr, ok := err.(*discordgo.RESTError); ok {
			if (restErr.Message != nil && restErr.Message.Code == discordgo.ErrCodeUnknownMessage) ||
				(restErr.Response != nil && restErr.Response.StatusCode == http.StatusNotFound) {
				return ErrUnknownMessage
			}
		}
		return err
	}
	return nil
}

// SessionAPI captures the methods used for Discord messaging so they can be mocked.
// ErrUnknownMessage should be used by tests to simulate a 404/UnknownMessage case.
type SessionAPI interface {
	Check() error
	ChannelMessage(channelID, messageID string) (ConcreteMessage, error)
	ChannelMessageSend(channelID, content string) (ConcreteMessage, error)
	ChannelMessageEdit(channelID, messageID, content string) error
	ChannelMessageDelete(channelID, messageID string) error
}
