package discord

import (
	"fmt"
	"slugbot/internal/io/slog"
)

type Message struct {
	API                SessionAPI
	ChannelID          string
	MessageID          string
	RepliedToMessageID string
}

// Create a new unsent Message
func NewMessage(api SessionAPI, channelID string, repliedToMessageID string) (*Message, error) {
	if err := api.Check(); err != nil {
		return nil, fmt.Errorf("NewMessage: encountered error: %w", err)
	}
	if channelID == "" {
		return nil, fmt.Errorf("NewMessage: received empty channelID string")
	}
	if repliedToMessageID == "" {
		return nil, fmt.Errorf("NewMessage: received empty ID for message to reply to")
	}
	return &Message{
		API:                api,
		ChannelID:          channelID,
		MessageID:          "",
		RepliedToMessageID: repliedToMessageID,
	}, nil
}

// Send an initial message, keeping track of its MessageID for updating later
func (m *Message) Create(messageContent string) error {
	if err := m.API.Check(); err != nil {
		return fmt.Errorf("Create failed validation: encountered error: %w", err)
	}
	if m.ChannelID == "" {
		return fmt.Errorf("Create failed validation: unset channel ID")
	}
	if m.MessageID != "" {
		return fmt.Errorf("Create failed validation: message ID is already set")
	}
	if m.RepliedToMessageID == "" {
		return fmt.Errorf("Create failed validation: ID of message to reply to is unset")
	}

	msg, err := m.API.ChannelMessageSendReply(m.ChannelID, messageContent, m.RepliedToMessageID)
	if err != nil {
		return fmt.Errorf("Create request: encountered error: %w", err)
	}

	m.MessageID = msg.ID

	return nil
}

// Updates a message with new content, provided `Create()` has been called first
func (m *Message) Update(messageContent string) error {
	if err := m.validate(); err != nil {
		return fmt.Errorf("Update validation: encountered error: %w", err)
	}

	err := m.API.ChannelMessageEdit(m.ChannelID, m.MessageID, messageContent)
	if err != nil {
		return fmt.Errorf("Update request: encountered error: %w", err)
	}

	return nil
}

// Deletes the associated message and remove its association to the object
func (m *Message) Delete() error {
	if err := m.validate(); err != nil {
		return fmt.Errorf("Delete validation: encountered error: %w", err)
	}

	err := m.API.ChannelMessageDelete(m.ChannelID, m.MessageID)
	if err == ErrUnknownMessage {
		slog.Warn("Delete: message already gone")
		m.MessageID = ""
		return nil
	}
	if err != nil {
		return fmt.Errorf("Delete request: encountered error: %w", err)
	}

	m.MessageID = ""
	return nil
}

// validates that the message is completely initialized
func (m *Message) validate() error {
	if err := m.API.Check(); err != nil {
		return fmt.Errorf("uninitialized session")
	}
	if m.ChannelID == "" {
		return fmt.Errorf("empty channel ID")
	}
	if m.MessageID == "" {
		return fmt.Errorf("empty message ID")
	}

	return nil
}
