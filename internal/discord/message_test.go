package discord

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeAPI struct {
	CheckError            error
	MsgReturnedFromGet    ConcreteMessage
	GetError              error
	MsgReturnedFromCreate ConcreteMessage
	CreateError           error
	EditError             error
	DeleteError           error
	data                  receivedData
}

type receivedData struct {
	calls [][]string
}

func (f *fakeAPI) Check() error {
	return f.CheckError
}
func (f *fakeAPI) ChannelMessage(channelID string, messageID string) (ConcreteMessage, error) {
	f.data.calls = append(f.data.calls, []string{"ChannelMessage", channelID, messageID})
	return f.MsgReturnedFromGet, f.GetError
}
func (f *fakeAPI) ChannelMessageSend(channelID string, content string) (ConcreteMessage, error) {
	f.data.calls = append(f.data.calls, []string{"ChannelMessageSend", channelID, content})
	return f.MsgReturnedFromCreate, f.CreateError
}
func (f *fakeAPI) ChannelMessageSendReply(channelID string, content string, replyToID string) (ConcreteMessage, error) {
	f.data.calls = append(f.data.calls, []string{"ChannelMessageSendReply", channelID, content, replyToID})
	return f.MsgReturnedFromCreate, f.CreateError
}
func (f *fakeAPI) ChannelMessageEdit(channelID string, messageID string, content string) error {
	f.data.calls = append(f.data.calls, []string{"ChannelMessageEdit", channelID, messageID, content})
	return f.EditError
}
func (f *fakeAPI) ChannelMessageDelete(channelID string, messageID string) error {
	f.data.calls = append(f.data.calls, []string{"ChannelMessageDelete", channelID, messageID})
	return f.DeleteError
}

// NewMessage tests
func TestNewMessage_Success(t *testing.T) {
	api := &fakeAPI{CheckError: nil}

	m, err := NewMessage(api, "chan", "replied")
	require.NoError(t, err)
	require.Equal(t, "chan", m.ChannelID)
	require.Equal(t, "replied", m.RepliedToMessageID)
}

func TestNewMessage_NilSession(t *testing.T) {
	api := &fakeAPI{CheckError: errors.New("invalid")}

	_, err := NewMessage(api, "chan", "replied")
	require.Error(t, err)
}

func TestNewMessage_EmptyChannelID(t *testing.T) {
	api := &fakeAPI{CheckError: nil}

	_, err := NewMessage(api, "", "replied")
	require.Error(t, err)
}

func TestNewMessage_EmptyReplyMessageID(t *testing.T) {
	api := &fakeAPI{CheckError: nil}

	_, err := NewMessage(api, "chan", "")
	require.Error(t, err)
}

// Message.Create tests
func TestCreate_Success(t *testing.T) {
	api := &fakeAPI{CheckError: nil, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: "sent123"}}
	m, _ := NewMessage(api, "chan", "replied")

	require.Equal(t, "", m.MessageID)
	err := m.Create("hello")
	require.NoError(t, err)
	require.Equal(t, "sent123", m.MessageID)

	require.Equal(t, 1, len(api.data.calls))
	require.Equal(t, "ChannelMessageSendReply", api.data.calls[0][0])
	require.Equal(t, []string{"ChannelMessageSendReply", "chan", "hello", "replied"}, api.data.calls[0])
}

func TestCreate_InvalidSession(t *testing.T) {
	api := &fakeAPI{CheckError: nil, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: "sent123"}}
	m, _ := NewMessage(api, "chan", "replied")
	api.CheckError = errors.New("invalid")

	err := m.Create("content")
	require.Error(t, err)

	require.Equal(t, 0, len(api.data.calls))
}

func TestCreate_EmptyChannelID(t *testing.T) {
	api := &fakeAPI{CheckError: nil}
	m, _ := NewMessage(api, "chan", "replied")
	m.ChannelID = ""

	err := m.Create("content")
	require.Error(t, err)

	require.Equal(t, 0, len(api.data.calls))
}

func TestCreate_AlreadyHasMessageID(t *testing.T) {
	api := &fakeAPI{CheckError: nil}
	m, _ := NewMessage(api, "chan", "replied")
	m.MessageID = "abcde"

	err := m.Create("content")
	require.Error(t, err)

	require.Equal(t, 0, len(api.data.calls))
}

func TestCreate_EmptyReplyToMessageID(t *testing.T) {
	api := &fakeAPI{CheckError: nil}
	m, _ := NewMessage(api, "chan", "replied")
	m.RepliedToMessageID = ""

	err := m.Create("content")
	require.Error(t, err)

	require.Equal(t, 0, len(api.data.calls))
}

func TestCreate_CreateError(t *testing.T) {
	api := &fakeAPI{CheckError: nil, CreateError: errors.New("fail")}
	m, _ := NewMessage(api, "chan", "replied")

	err := m.Create("content")
	require.Error(t, err)

	require.Equal(t, 1, len(api.data.calls))
	require.Equal(t, "ChannelMessageSendReply", api.data.calls[0][0])
}

// Message.Update tests
func TestUpdate_Success(t *testing.T) {
	channelID := "channel-id"
	repliedMsgID := "replied-msg-id"
	createdMsgID := "created-msg-id"
	initialContent := "initial-content-str"
	updatedContent := "updated-content-str"

	api := &fakeAPI{CheckError: nil, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: createdMsgID}}
	m, _ := NewMessage(api, channelID, repliedMsgID)

	_ = m.Create(initialContent)
	err := m.Update(updatedContent)
	require.NoError(t, err)

	require.Equal(t, 2, len(api.data.calls))
	require.Equal(t, []string{"ChannelMessageSendReply", channelID, initialContent, repliedMsgID}, api.data.calls[0])
	require.Equal(t, []string{"ChannelMessageEdit", channelID, createdMsgID, updatedContent}, api.data.calls[1])
}

func TestUpdate_InvalidSession(t *testing.T) {
	channelID := "channel-id"
	repliedMsgID := "replied-msg-id"
	createdMsgID := "created-msg-id"
	initialContent := "initial-content-str"
	updatedContent := "updated-content-str"

	api := &fakeAPI{CheckError: nil, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: createdMsgID}}
	m, _ := NewMessage(api, channelID, repliedMsgID)
	_ = m.Create(initialContent)

	api.CheckError = errors.New("invalid api")

	err := m.Update(updatedContent)
	require.Error(t, err)
}

func TestUpdate_EmptyChannelID(t *testing.T) {
	channelID := "channel-id"
	repliedMsgID := "replied-msg-id"
	createdMsgID := "created-msg-id"
	initialContent := "initial-content-str"
	updatedContent := "updated-content-str"

	api := &fakeAPI{CheckError: nil, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: createdMsgID}}
	m, _ := NewMessage(api, channelID, repliedMsgID)
	_ = m.Create(initialContent)

	m.ChannelID = ""

	err := m.Update(updatedContent)
	require.Error(t, err)
}

func TestUpdate_ValidateMessageID(t *testing.T) {
	channelID := "channel-id"
	repliedMsgID := "replied-msg-id"
	createdMsgID := "created-msg-id"
	initialContent := "initial-content-str"
	updatedContent := "updated-content-str"

	api := &fakeAPI{CheckError: nil, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: createdMsgID}}
	m, _ := NewMessage(api, channelID, repliedMsgID)
	_ = m.Create(initialContent)

	m.MessageID = ""

	err := m.Update(updatedContent)
	require.Error(t, err)
}

func TestUpdate_EditError(t *testing.T) {
	channelID := "channel-id"
	repliedMsgID := "replied-msg-id"
	createdMsgID := "created-msg-id"
	initialContent := "initial-content-str"
	updatedContent := "updated-content-str"

	api := &fakeAPI{CheckError: nil, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: createdMsgID}}
	m, _ := NewMessage(api, channelID, repliedMsgID)
	_ = m.Create(initialContent)

	api.EditError = errors.New("fail editing message")

	err := m.Update(updatedContent)
	require.Error(t, err)

	require.Equal(t, 2, len(api.data.calls))
}

// Message.Delete tests
func TestDelete_Success(t *testing.T) {
	channelID := "channel-id"
	repliedMsgID := "replied-msg-id"
	createdMsgID := "created-msg-id"
	initialContent := "initial-content-str"
	updatedContent := "updated-content-str"

	api := &fakeAPI{CheckError: nil, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: createdMsgID}}
	m, _ := NewMessage(api, channelID, repliedMsgID)
	_ = m.Create(initialContent)
	_ = m.Update(updatedContent)

	err := m.Delete()
	require.NoError(t, err)

	require.Equal(t, "", m.MessageID)
	require.Equal(t, 3, len(api.data.calls))
	require.Equal(t, []string{"ChannelMessageSendReply", channelID, initialContent, repliedMsgID}, api.data.calls[0])
	require.Equal(t, []string{"ChannelMessageEdit", channelID, createdMsgID, updatedContent}, api.data.calls[1])
	require.Equal(t, []string{"ChannelMessageDelete", channelID, createdMsgID}, api.data.calls[2])
}

func TestDelete_InvalidSession(t *testing.T) {
	channelID := "channel-id"
	repliedMsgID := "replied-msg-id"
	createdMsgID := "created-msg-id"
	initialContent := "initial-content-str"
	updatedContent := "updated-content-str"

	api := &fakeAPI{CheckError: nil, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: createdMsgID}}
	m, _ := NewMessage(api, channelID, repliedMsgID)
	_ = m.Create(initialContent)
	_ = m.Update(updatedContent)

	api.CheckError = errors.New("invalid-session-error")

	err := m.Delete()
	require.Error(t, err)
}

func TestDelete_EmptyChannelID(t *testing.T) {
	channelID := "channel-id"
	repliedMsgID := "replied-msg-id"
	createdMsgID := "created-msg-id"
	initialContent := "initial-content-str"
	updatedContent := "updated-content-str"

	api := &fakeAPI{CheckError: nil, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: createdMsgID}}
	m, _ := NewMessage(api, channelID, repliedMsgID)
	_ = m.Create(initialContent)
	_ = m.Update(updatedContent)

	m.ChannelID = ""

	err := m.Delete()
	require.Error(t, err)
}

func TestDelete_EmptyMessageID(t *testing.T) {
	channelID := "channel-id"
	repliedMsgID := "replied-msg-id"
	createdMsgID := "created-msg-id"
	initialContent := "initial-content-str"
	updatedContent := "updated-content-str"

	api := &fakeAPI{CheckError: nil, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: createdMsgID}}
	m, _ := NewMessage(api, channelID, repliedMsgID)
	_ = m.Create(initialContent)
	_ = m.Update(updatedContent)

	m.MessageID = ""

	err := m.Delete()
	require.Error(t, err)
}

func TestDelete_NotFound(t *testing.T) {
	channelID := "channel-id"
	repliedMsgID := "replied-msg-id"
	createdMsgID := "created-msg-id"
	initialContent := "initial-content-str"
	updatedContent := "updated-content-str"

	api := &fakeAPI{CheckError: nil, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: createdMsgID}, DeleteError: ErrUnknownMessage}
	m, _ := NewMessage(api, channelID, repliedMsgID)
	_ = m.Create(initialContent)
	_ = m.Update(updatedContent)

	require.Equal(t, createdMsgID, m.MessageID)
	err := m.Delete()
	require.NoError(t, err)
	require.Equal(t, "", m.MessageID)
}

func TestDelete_OtherDeleteError(t *testing.T) {
	channelID := "channel-id"
	repliedMsgID := "replied-msg-id"
	createdMsgID := "created-msg-id"
	initialContent := "initial-content-str"
	updatedContent := "updated-content-str"

	api := &fakeAPI{CheckError: nil, CreateError: nil, MsgReturnedFromCreate: ConcreteMessage{ID: createdMsgID}, DeleteError: errors.New("non-404-delete-error")}
	m, _ := NewMessage(api, channelID, repliedMsgID)
	_ = m.Create(initialContent)
	_ = m.Update(updatedContent)

	require.Equal(t, createdMsgID, m.MessageID)
	err := m.Delete()
	require.Error(t, err)
	require.Equal(t, createdMsgID, m.MessageID)
}
