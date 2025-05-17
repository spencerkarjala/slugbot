package audio

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Poller interface {
	Start(done <-chan struct{})
}

// PollableFile watches a file at regular intervals and invokes OnUpdate with the trimmed content.
type PollableFile struct {
	File     string            // Path to the file being watched
	Interval time.Duration     // Polling interval
	OnUpdate func(text string) // Callback invoked on each update
}

// NewPollableFile creates a PollableFile with a unique temporary file, polling interval, and update callback.
// The temporary file is created in the OS temp directory with a unique name.
func NewPollableFile(interval time.Duration, onUpdate func(string)) (*PollableFile, error) {
	if onUpdate == nil {
		return nil, fmt.Errorf("received nil onUpdate callback")
	}
	tmpFile, err := os.CreateTemp("", "pollable-*.progress")
	if err != nil {
		return nil, err
	}
	tmpFile.Close()
	return &PollableFile{File: tmpFile.Name(), Interval: interval, OnUpdate: onUpdate}, nil
}

// Start polls the file until done is closed, calling OnUpdate on each non-empty read.
func (pf *PollableFile) Start(done <-chan struct{}) {
	ticker := time.NewTicker(pf.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			data, err := os.ReadFile(pf.File)
			if err != nil {
				continue
			}
			text := strings.TrimSpace(string(data))
			if text != "" && pf.OnUpdate != nil {
				pf.OnUpdate(text)
			}
		}
	}
}
