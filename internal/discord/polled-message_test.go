package discord

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockSessionAPI struct {
	CheckError       error
	CreateError      error
	EditError        error
	DeleteError      error
	CreatedMessageID string
	data             receivedAPIData
}

type receivedAPIData struct {
	calls [][]string
}

func (f *mockSessionAPI) Check() error {
	return f.CheckError
}

func (f *mockSessionAPI) ChannelMessage(channelID, messageID string) (ConcreteMessage, error) {
	return ConcreteMessage{}, nil
}

func (f *mockSessionAPI) ChannelMessageSend(channelID, content string) (ConcreteMessage, error) {
	f.data.calls = append(f.data.calls, []string{"ChannelMessageSend", channelID, content})
	return ConcreteMessage{ID: f.CreatedMessageID}, f.CreateError
}

func (f *mockSessionAPI) ChannelMessageSendReply(channelID, content, replyToID string) (ConcreteMessage, error) {
	f.data.calls = append(f.data.calls, []string{"ChannelMessageSendReply", channelID, content, replyToID})
	return ConcreteMessage{ID: f.CreatedMessageID}, f.CreateError
}

func (f *mockSessionAPI) ChannelMessageEdit(channelID, messageID, content string) error {
	f.data.calls = append(f.data.calls, []string{"ChannelMessageEdit", channelID, messageID, content})
	return f.EditError
}

func (f *mockSessionAPI) ChannelMessageDelete(channelID, messageID string) error {
	f.data.calls = append(f.data.calls, []string{"ChannelMessageDelete", channelID, messageID})
	return f.DeleteError
}

// Test constructor
func TestNewFilePollMessage_Success(t *testing.T) {
	channelID := "test-channel-id"
	repliedToMessageID := "test-replied-to-msg-id"
	api := &mockSessionAPI{CheckError: nil}

	fpm, err := NewFilePollMessage(api, channelID, repliedToMessageID, time.Millisecond)
	require.NoError(t, err)
	require.NotEmpty(t, fpm.FilePath)
}

func TestNewFilePollMessage_SessionError(t *testing.T) {
	channelID := "test-channel-id"
	repliedToMessageID := "test-replied-to-msg-id"
	api := &mockSessionAPI{CheckError: errors.New("bad")}

	fpm, err := NewFilePollMessage(api, channelID, repliedToMessageID, time.Millisecond)
	require.Error(t, err)
	require.Empty(t, fpm)
}

func TestNewFilePollMessage_EmptyChannelID(t *testing.T) {
	channelID := ""
	repliedToMessageID := "test-replied-to-msg-id"
	api := &mockSessionAPI{CheckError: nil}

	fpm, err := NewFilePollMessage(api, channelID, repliedToMessageID, time.Millisecond)
	require.Error(t, err)
	require.Empty(t, fpm)
}

func TestNewFilePollMessage_EmptyReplyID(t *testing.T) {
	channelID := "test-channel-id"
	repliedToMessageID := ""
	api := &mockSessionAPI{CheckError: nil}

	fpm, err := NewFilePollMessage(api, channelID, repliedToMessageID, time.Millisecond)
	require.Error(t, err)
	require.Empty(t, fpm)
}

func TestFilePollMessage_StartAndStopSuccess(t *testing.T) {
	channelID := "test-channel-id"
	repliedToMessageID := "test-replied-to-msg-id"
	messageID := "next-id-123"
	api := &mockSessionAPI{CheckError: nil, CreatedMessageID: messageID}
	initialContent := "initial-content"
	fpm, _ := NewFilePollMessage(api, channelID, repliedToMessageID, time.Millisecond)

	require.NoError(t, fpm.Start(initialContent))
	require.Len(t, api.data.calls, 1)
	require.Equal(t, []string{"ChannelMessageSendReply", channelID, initialContent, repliedToMessageID}, api.data.calls[0])
	require.Equal(t, messageID, fpm.Message.MessageID)

	shmFile, err := os.Stat(fpm.FilePath)
	require.NoError(t, err)
	require.False(t, shmFile.IsDir())

	require.NoError(t, fpm.Stop())
	require.Len(t, api.data.calls, 2)
	require.Equal(t, []string{"ChannelMessageDelete", channelID, messageID}, api.data.calls[1])
	require.Empty(t, fpm.Message.MessageID)
}

func TestFilePollMessage_SendsFileUpdatesToMessage(t *testing.T) {
	channelID := "test-channel-id"
	repliedToMessageID := "test-replied-to-msg-id"
	messageID := "next-id-123"
	api := &mockSessionAPI{CheckError: nil, CreatedMessageID: messageID}
	initialContent := "initial-content"
	updatedContent := []byte("updated-content")
	interval := 30 * time.Millisecond

	fpm, _ := NewFilePollMessage(api, channelID, repliedToMessageID, interval)
	_ = fpm.Start(initialContent)

	time.Sleep(interval / 2)

	require.NoError(t, os.WriteFile(fpm.FilePath, updatedContent, 0644))

	time.Sleep(interval)

	require.NoError(t, fpm.Stop())
	require.Len(t, api.data.calls, 3)
	require.Equal(t, []string{"ChannelMessageSendReply", channelID, initialContent, repliedToMessageID}, api.data.calls[0])
	require.Equal(t, []string{"ChannelMessageEdit", channelID, messageID, string(updatedContent)}, api.data.calls[1])
	require.Equal(t, []string{"ChannelMessageDelete", channelID, messageID}, api.data.calls[2])
}
