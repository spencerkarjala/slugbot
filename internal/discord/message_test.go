// Code in message.go assumes Create and validate use `err != nil` checks on API.Check. If still `err == nil`, please refactor:
//   if err := m.API.Check(); err != nil {
//       return fmt.Errorf("...: %w", err)
//   }

package discord

import (
	"errors"
	"testing"
)

type fakeAPI struct {
	CheckError            error
	MsgReturnedFromGet    ConcreteMessage
	GetError              error
	MsgReturnedFromCreate ConcreteMessage
	CreateError           error
	EditError             error
	DeleteError           error
}

func (f *fakeAPI) Check() error {
	return f.CheckError
}
func (f *fakeAPI) ChannelMessage(channelID string, messageID string) (ConcreteMessage, error) {
	return f.MsgReturnedFromGet, f.GetError
}
func (f *fakeAPI) ChannelMessageSend(channelID string, content string) (ConcreteMessage, error) {
	return f.MsgReturnedFromCreate, f.CreateError
}
func (f *fakeAPI) ChannelMessageSendReply(channelID string, content string, replyToID string) (ConcreteMessage, error) {
	return f.MsgReturnedFromCreate, f.CreateError
}
func (f *fakeAPI) ChannelMessageEdit(channelID string, messageID, content string) error {
	return f.EditError
}
func (f *fakeAPI) ChannelMessageDelete(channelID string, messageID string) error {
	return f.DeleteError
}

// NewMessage tests
func TestNewMessage_Success(t *testing.T) {
	api := &fakeAPI{CheckError: nil}
	m, err := NewMessage(api, "chan")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if m.ChannelID != "chan" {
		t.Errorf("expected ChannelID 'chan', got %q", m.ChannelID)
	}
}

func TestNewMessage_NilSession(t *testing.T) {
	api := &fakeAPI{CheckError: errors.New("invalid")}
	if _, err := NewMessage(api, "chan"); err == nil {
		t.Fatal("expected error for invalid session, got nil")
	}
}

func TestNewMessage_EmptyChannelID(t *testing.T) {
	api := &fakeAPI{CheckError: nil}
	if _, err := NewMessage(api, ""); err == nil {
		t.Fatal("expected error for empty channelID, got nil")
	}
}

// Message.Create tests
func TestCreate_Success(t *testing.T) {
	api := &fakeAPI{CheckError: nil, GetError: ErrUnknownMessage, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: "sent123"}}
	m := &Message{API: api, ChannelID: "chan", MessageID: ""}
	if err := m.Create("hello"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if m.MessageID != "sent123" {
		t.Errorf("expected MessageID 'sent123', got %q", m.MessageID)
	}
}

func TestCreate_InvalidSession(t *testing.T) {
	api := &fakeAPI{CheckError: errors.New("invalid")}
	m := &Message{API: api, ChannelID: "chan", MessageID: ""}
	if err := m.Create("x"); err == nil {
		t.Fatal("expected validation error for session, got nil")
	}
}

func TestCreate_EmptyChannelID(t *testing.T) {
	api := &fakeAPI{CheckError: nil}
	m := &Message{API: api, ChannelID: "", MessageID: ""}
	if err := m.Create("x"); err == nil {
		t.Fatal("expected error for empty channelID, got nil")
	}
}

func TestCreate_AlreadyHasMessageID(t *testing.T) {
	api := &fakeAPI{CheckError: nil}
	m := &Message{API: api, ChannelID: "chan", MessageID: "msg123"}
	if err := m.Create("x"); err == nil {
		t.Fatal("expected error for already-set MessageID, got nil")
	}
}

func TestCreate_ExistsTrue(t *testing.T) {
	api := &fakeAPI{CheckError: nil, GetError: nil}
	m := &Message{API: api, ChannelID: "chan", MessageID: "msg123"}
	// exists calls ChannelMessage only when IDs are set; to simulate exist-true, set both IDs
	m.ChannelID, m.MessageID = "chan", "msg123"
	if _, err := m.exists(); err != nil {
		t.Fatalf("setup failure: %v", err)
	}
	// Now test Create on message that exists
	m.MessageID = ""
	api.GetError = nil // ChannelMessage returns no error => exists true
	if err := m.Create("x"); err == nil {
		t.Fatal("expected exists-true error, got nil")
	}
}

func TestCreate_ExistsError(t *testing.T) {
	api := &fakeAPI{CheckError: nil, GetError: errors.New("boom")}
	m := &Message{API: api, ChannelID: "chan", MessageID: "msg123"}
	if err := m.Create("x"); err == nil {
		t.Fatal("expected exists-error, got nil")
	}
}

func TestCreate_CreateErroror(t *testing.T) {
	api := &fakeAPI{CheckError: nil, GetError: ErrUnknownMessage, CreateError: errors.New("fail")}
	m := &Message{API: api, ChannelID: "chan", MessageID: ""}
	if err := m.Create("hi"); err == nil {
		t.Fatal("expected send-error, got nil")
	}
}

// Message.Update tests
func TestUpdate_Success(t *testing.T) {
	api := &fakeAPI{CheckError: nil, EditError: nil}
	m := &Message{API: api, ChannelID: "chan", MessageID: "msg"}
	if err := m.Update("new"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestUpdate_ValidateSession(t *testing.T) {
	api := &fakeAPI{CheckError: errors.New("invalid")}
	m := &Message{API: api, ChannelID: "chan", MessageID: "msg"}
	if err := m.Update("x"); err == nil {
		t.Fatal("expected session validation error, got nil")
	}
}

func TestUpdate_ValidateChannelID(t *testing.T) {
	api := &fakeAPI{CheckError: nil}
	m := &Message{API: api, ChannelID: "", MessageID: "msg"}
	if err := m.Update("x"); err == nil {
		t.Fatal("expected channelID validation error, got nil")
	}
}

func TestUpdate_ValidateMessageID(t *testing.T) {
	api := &fakeAPI{CheckError: nil}
	m := &Message{API: api, ChannelID: "chan", MessageID: ""}
	if err := m.Update("x"); err == nil {
		t.Fatal("expected messageID validation error, got nil")
	}
}

func TestUpdate_EditError(t *testing.T) {
	api := &fakeAPI{CheckError: nil, EditError: errors.New("fail")}
	m := &Message{API: api, ChannelID: "chan", MessageID: "msg"}
	if err := m.Update("x"); err == nil {
		t.Fatal("expected edit-error, got nil")
	}
}

// Message.Delete tests
func TestDelete_Success(t *testing.T) {
	api := &fakeAPI{CheckError: nil, DeleteError: nil}
	m := &Message{API: api, ChannelID: "chan", MessageID: "msg"}
	if err := m.Delete(); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if m.MessageID != "" {
		t.Errorf("expected MessageID cleared, got %q", m.MessageID)
	}
}

func TestDelete_ValidateSession(t *testing.T) {
	api := &fakeAPI{CheckError: errors.New("invalid")}
	m := &Message{API: api, ChannelID: "chan", MessageID: "msg"}
	if err := m.Delete(); err == nil {
		t.Fatal("expected session validation error, got nil")
	}
}

func TestDelete_ValidateChannelID(t *testing.T) {
	api := &fakeAPI{CheckError: nil}
	m := &Message{API: api, ChannelID: "", MessageID: "msg"}
	if err := m.Delete(); err == nil {
		t.Fatal("expected channelID validation error, got nil")
	}
}

func TestDelete_ValidateMessageID(t *testing.T) {
	api := &fakeAPI{CheckError: nil}
	m := &Message{API: api, ChannelID: "chan", MessageID: ""}
	if err := m.Delete(); err == nil {
		t.Fatal("expected messageID validation error, got nil")
	}
}

func TestDelete_NotFound(t *testing.T) {
	api := &fakeAPI{CheckError: nil, DeleteError: ErrUnknownMessage}
	m := &Message{API: api, ChannelID: "chan", MessageID: "msg"}
	if err := m.Delete(); err != nil {
		t.Fatalf("expected success on unknown message, got %v", err)
	}
	if m.MessageID != "" {
		t.Errorf("expected MessageID cleared after not-found, got %q", m.MessageID)
	}
}

func TestDelete_OtherDeleteError(t *testing.T) {
	api := &fakeAPI{CheckError: nil, DeleteError: errors.New("boom")}
	m := &Message{API: api, ChannelID: "chan", MessageID: "msg"}
	if err := m.Delete(); err == nil {
		t.Fatal("expected delete error, got nil")
	}
}

// Message.exists tests
func TestExists_NoIDs(t *testing.T) {
	api := &fakeAPI{CheckError: nil}
	m := &Message{API: api, ChannelID: "", MessageID: ""}
	exists, err := m.exists()
	if err != nil || exists {
		t.Fatalf("expected false,nil; got %v, %v", exists, err)
	}
}

func TestExists_Success(t *testing.T) {
	api := &fakeAPI{CheckError: nil, GetError: nil, MsgReturnedFromGet: ConcreteMessage{ID: "msg"}}
	m := &Message{API: api, ChannelID: "chan", MessageID: "msg"}
	exists, err := m.exists()
	if err != nil || !exists {
		t.Fatalf("expected true,nil; got %v, %v", exists, err)
	}
}

func TestExists_UnknownMessage(t *testing.T) {
	api := &fakeAPI{CheckError: nil, GetError: ErrUnknownMessage}
	m := &Message{API: api, ChannelID: "chan", MessageID: "msg"}
	exists, err := m.exists()
	if err != nil || exists {
		t.Fatalf("expected false,nil; got %v, %v", exists, err)
	}
}

func TestExists_OtherError(t *testing.T) {
	api := &fakeAPI{CheckError: nil, GetError: errors.New("boom")}
	m := &Message{API: api, ChannelID: "chan", MessageID: "msg"}
	exists, err := m.exists()
	if err == nil || exists {
		t.Fatalf("expected error,false; got %v, %v", exists, err)
	}
}
