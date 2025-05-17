package discord

import (
	"os"
	"sync"
	"testing"
	"time"
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
func (m *fakeDiscordAPI) ChannelMessageEdit(channelID, messageID, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EditsSetByTest = append(m.EditsSetByTest, content)
	return m.EditError
}
func (m *fakeDiscordAPI) ChannelMessageDelete(channelID, messageID string) error {
	return m.DeleteError
}

func TestFileCreated(t *testing.T) {
	api := &fakeDiscordAPI{}
	fpm, err := NewFilePollMessage(api, "chan", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("NewFilePollMessage: %v", err)
	}
	info, err := os.Stat(fpm.FilePath)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if info.IsDir() {
		t.Fatalf("expected file, got dir")
	}
}

func TestStartSendsInitialMessage(t *testing.T) {
	api := &fakeDiscordAPI{
		MsgReturnedFromCreate: ConcreteMessage{ID: "m1"}, CreateError: nil, GetError: ErrUnknownMessage,
	}
	fpm, err := NewFilePollMessage(api, "abc", 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := fpm.Start("hello"); err != nil {
		t.Fatal(err)
	}
	if api.ChannelSetByPolledFileOnCreate != "abc" || api.ContentSetByPolledFileOnCreate != "hello" {
		t.Errorf("initial create got %q,%q; want abc,hello", api.ChannelSetByPolledFileOnCreate, api.ContentSetByPolledFileOnCreate)
	}
	if fpm.Message.MessageID != "m1" {
		t.Errorf("MessageID = %q; want m1", fpm.Message.MessageID)
	}
	fpm.Stop()
}

func TestSingleUpdate(t *testing.T) {
	interval := 30 * time.Millisecond
	api := &fakeDiscordAPI{MsgReturnedFromCreate: ConcreteMessage{ID: "m2"}, CreateError: nil, GetError: ErrUnknownMessage}
	fpm, err := NewFilePollMessage(api, "x", interval)
	if err != nil {
		t.Fatal(err)
	}
	if err := fpm.Start("init"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(interval / 2)
	if err := os.WriteFile(fpm.FilePath, []byte("upd1"), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(interval)
	fpm.Stop()
	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.EditsSetByTest) != 1 {
		t.Fatalf("edits = %d; want 1", len(api.EditsSetByTest))
	}
	if api.EditsSetByTest[0] != "upd1" {
		t.Errorf("edits[0] = %q; want upd1", api.EditsSetByTest[0])
	}
}

func TestRapidWritesCollapse(t *testing.T) {
	interval := 30 * time.Millisecond
	api := &fakeDiscordAPI{MsgReturnedFromCreate: ConcreteMessage{ID: "m3"}, CreateError: nil, GetError: ErrUnknownMessage}
	fpm, err := NewFilePollMessage(api, "y", interval)
	if err != nil {
		t.Fatal(err)
	}
	if err := fpm.Start("start"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(interval / 3)
	os.WriteFile(fpm.FilePath, []byte("one"), 0644)
	time.Sleep(interval / 3)
	os.WriteFile(fpm.FilePath, []byte("two"), 0644)
	time.Sleep(interval)
	fpm.Stop()
	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.EditsSetByTest) != 1 {
		t.Fatalf("edits = %d; want 1", len(api.EditsSetByTest))
	}
	if api.EditsSetByTest[0] != "two" {
		t.Errorf("edits[0] = %q; want two", api.EditsSetByTest[0])
	}
}
