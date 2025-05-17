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

// fetches a single message. It wraps Discord REST errors into ErrUnknownMessage when appropriate.
func (api ConcreteSession) ChannelMessage(channelID string, messageID string) (ConcreteMessage, error) {
	msg, err := getMessage(api.Session, channelID, messageID)
	if err != nil {
		return ConcreteMessage{}, err
	}

	return ConcreteMessage{ID: msg.ID}, nil
}

// sends a new message to the channel. Errors are passed through directly.
func (api ConcreteSession) ChannelMessageSend(channelID string, content string) (ConcreteMessage, error) {
	msg, err := api.Session.ChannelMessageSend(channelID, content)
	if err != nil {
		return ConcreteMessage{}, err
	}
	return ConcreteMessage{ID: msg.ID}, nil
}

// sends a new message to the channel, replying to a specific message.
func (api ConcreteSession) ChannelMessageSendReply(channelID string, content string, replyToMessageID string) (ConcreteMessage, error) {
	messageToReplyTo, err := getMessage(api.Session, channelID, replyToMessageID)
	if err != nil {
		return ConcreteMessage{}, err
	}

	msg, err := api.Session.ChannelMessageSendReply(channelID, content, messageToReplyTo.Reference())
	if err != nil {
		return ConcreteMessage{}, err
	}

	return ConcreteMessage{ID: msg.ID}, nil
}

// edits an existing message’s content. Errors are passed through directly.
func (api ConcreteSession) ChannelMessageEdit(channelID string, messageID, content string) error {
	_, err := api.Session.ChannelMessageEdit(channelID, messageID, content)
	return err
}

// deletes the specified message. It wraps Discord REST errors into ErrUnknownMessage when appropriate.
func (api ConcreteSession) ChannelMessageDelete(channelID string, messageID string) error {
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

// captures the methods used for Discord messaging so they can be mocked.
// ErrUnknownMessage should be used by tests to simulate a 404/UnknownMessage case.
type SessionAPI interface {
	Check() error
	ChannelMessage(channelID string, messageID string) (ConcreteMessage, error)
	ChannelMessageSend(channelID string, content string) (ConcreteMessage, error)
	ChannelMessageSendReply(channelID string, content string, replyToID string) (ConcreteMessage, error)
	ChannelMessageEdit(channelID string, messageID, content string) error
	ChannelMessageDelete(channelID string, messageID string) error
}

// helper to get a message using only its string id
func getMessage(session *discordgo.Session, channelID string, messageID string) (*discordgo.Message, error) {
	msg, err := session.ChannelMessage(channelID, messageID)
	if err != nil {
		if restErr, ok := err.(*discordgo.RESTError); ok {
			if (restErr.Message != nil && restErr.Message.Code == discordgo.ErrCodeUnknownMessage) ||
				(restErr.Response != nil && restErr.Response.StatusCode == http.StatusNotFound) {
				return nil, ErrUnknownMessage
			}
		}
		return nil, err
	}
	return msg, nil
}
