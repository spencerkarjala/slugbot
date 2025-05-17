package discord

import (
	"fmt"
	"slugbot/internal/io/slog"
)

type Message struct {
	API       SessionAPI
	ChannelID string
	MessageID string
}

// Create a new unsent Message
func NewMessage(api SessionAPI, channelID string) (*Message, error) {
	if err := api.Check(); err != nil {
		return nil, fmt.Errorf("NewMessage: encountered error: %w", err)
	}
	if channelID == "" {
		return nil, fmt.Errorf("NewMessage: received empty channelID string")
	}
	return &Message{
		API:       api,
		ChannelID: channelID,
		MessageID: "",
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

	messageExists, err := m.exists()
	if err != nil {
		return fmt.Errorf("Create existence check: encountered error: %w", err)
	} else if messageExists {
		return fmt.Errorf("Create existence check: message already exists")
	}

	msg, err := m.API.ChannelMessageSend(m.ChannelID, messageContent)
	fmt.Printf("%s\n", msg.ID)
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

func (m *Message) exists() (bool, error) {
	if err := m.API.Check(); err != nil {
		return false, fmt.Errorf("Message existence check: encountered error: %w", err)
	}
	// short-circuit when no ID yet
	if m.ChannelID == "" {
		return false, nil
	}

	_, err := m.API.ChannelMessage(m.ChannelID, m.MessageID)
	if err == ErrUnknownMessage {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("Message existence check: encountered error: %w", err)
	}
	return true, nil
}
