package discord

import (
	"sync"
)

type fakeDiscordAPI struct {
	CheckError                     error
	MsgReturnedFromGet             ConcreteMessage
	GetError                       error
	MsgReturnedFromCreate          ConcreteMessage
	CreateError                    error
	EditError                      error
	DeleteError                    error
	mu                             sync.Mutex
	EditsSetByTest                 []string
	ChannelSetByPolledFileOnCreate string
	ContentSetByPolledFileOnCreate string
}

func (m *fakeDiscordAPI) Check() error {
	return m.CheckError
}
func (m *fakeDiscordAPI) ChannelMessage(channelID, messageID string) (ConcreteMessage, error) {
	return m.MsgReturnedFromGet, m.GetError
}
func (m *fakeDiscordAPI) ChannelMessageSend(channelID, content string) (ConcreteMessage, error) {
	m.ChannelSetByPolledFileOnCreate = channelID
	m.ContentSetByPolledFileOnCreate = content
	return m.MsgReturnedFromCreate, m.CreateError
}
func (m *fakeDiscordAPI) ChannelMessageSendReply(channelID string, content string, replyToID string) (ConcreteMessage, error) {
	m.ChannelSetByPolledFileOnCreate = channelID
	m.ContentSetByPolledFileOnCreate = content
	return m.MsgReturnedFromCreate, m.CreateError
}
func (m *fakeDiscordAPI) ChannelMessageEdit(channelID, messageID, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EditsSetByTest = append(m.EditsSetByTest, content)
	return m.EditError
}
func (m *fakeDiscordAPI) ChannelMessageDelete(channelID, messageID string) error {
	return m.DeleteError
}
