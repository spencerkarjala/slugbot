package discord

import (
	"errors"
	"testing"
	"time"
)

type mockSessionAPI struct {
	CheckError   error
	SendID       string
	SendErr      error
	SendChan     string
	SendContent  string
	EditCalls    []string
	EditError    error
	DeleteCalled bool
	DeleteError  error
}

func (f *mockSessionAPI) Check() error {
	return f.CheckError
}
func (f *mockSessionAPI) ChannelMessage(channelID, messageID string) (ConcreteMessage, error) {
	return ConcreteMessage{}, ErrUnknownMessage
}
func (f *mockSessionAPI) ChannelMessageSend(channelID, content string) (ConcreteMessage, error) {
	f.SendChan, f.SendContent = channelID, content
	return ConcreteMessage{ID: f.SendID}, f.SendErr
}
func (f *mockSessionAPI) ChannelMessageEdit(channelID, messageID, content string) error {
	f.EditCalls = append(f.EditCalls, content)
	return f.EditError
}
func (f *mockSessionAPI) ChannelMessageDelete(channelID, messageID string) error {
	f.DeleteCalled = true
	return f.DeleteError
}

func TestNewFilePollMessage_Success(t *testing.T) {
	api := &mockSessionAPI{CheckError: nil}
	fpm, err := NewFilePollMessage(api, "chan", time.Millisecond)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if fpm.FilePath == "" {
		t.Error("expected non-empty FilePath")
	}
}

func TestNewFilePollMessage_SessionError(t *testing.T) {
	api := &mockSessionAPI{CheckError: errors.New("bad")}
	if _, err := NewFilePollMessage(api, "chan", time.Millisecond); err == nil {
		t.Error("expected session validation error")
	}
}

func TestNewFilePollMessage_EmptyChannelID(t *testing.T) {
	api := &mockSessionAPI{CheckError: nil}
	if _, err := NewFilePollMessage(api, "", time.Millisecond); err == nil {
		t.Error("expected empty-channelID error")
	}
}

func TestStartAndStop(t *testing.T) {
	api := &mockSessionAPI{CheckError: nil, SendID: "msg123"}
	fpm, err := NewFilePollMessage(api, "chan", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	// Start should send initial message
	if err := fpm.Start("init"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if api.SendChan != "chan" || api.SendContent != "init" {
		t.Errorf("Send called with %q,%q; want chan, init", api.SendChan, api.SendContent)
	}
	if fpm.Message.MessageID != "msg123" {
		t.Errorf("MessageID = %q; want msg123", fpm.Message.MessageID)
	}
	// Stop should delete and clear ID
	if err := fpm.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if !api.DeleteCalled {
		t.Error("expected delete to be called")
	}
	if fpm.Message.MessageID != "" {
		t.Errorf("MessageID = %q after Stop; want empty", fpm.Message.MessageID)
	}
}
