package audio

import (
	"os"
	"sync"
	"testing"
	"time"
)

func TestNewPollableFile(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		_, err := NewPollableFile(50*time.Millisecond, nil)
		if err == nil {
			t.Fatalf("Expected error when providing no callback to PollableFile")
		}

		if _, err := NewPollableFile(50*time.Millisecond, func(string) {}); err != nil {
			t.Fatalf("ran into unexpected error when creating default PollableFile: %v", err)
		}
	})
	t.Run("unique files and existence", func(t *testing.T) {
		testFile1, err := NewPollableFile(50*time.Millisecond, func(string) {})
		if err != nil {
			t.Fatalf("unexpected error creating first PollableFile: %v", err)
		}
		testFile2, err := NewPollableFile(50*time.Millisecond, func(string) {})
		if err != nil {
			t.Fatalf("unexpected error creating second PollableFile: %v", err)
		}

		if testFile1.File == testFile2.File {
			t.Errorf("expected unique file names, got same: %q", testFile1.File)
		}

		for _, pf := range []*PollableFile{testFile1, testFile2} {
			info, err := os.Stat(pf.File)
			if err != nil {
				t.Errorf("expected file %q to exist, got error: %v", pf.File, err)
				continue
			}
			if info.IsDir() {
				t.Errorf("expected %q to be a file, but it's a directory", pf.File)
			}
		}
	})

	t.Run("ConstructorError", func(t *testing.T) {
		// Simulate failure by overriding TMPDIR to invalid path
		orig := os.TempDir()
		t.Setenv("TMPDIR", "/nonexistent/path")

		if _, err := NewPollableFile(5*time.Millisecond, nil); err == nil {
			t.Error("expected error when temp dir is invalid, got nil")
		}

		t.Setenv("TMPDIR", orig)
	})
}

func TestPollableFile_Start(t *testing.T) {
	t.Run("SingleUpdateAndStop", func(t *testing.T) {
		testInterval := 50 * time.Millisecond

		var mu sync.Mutex
		var updates []string
		pf, err := NewPollableFile(testInterval, func(text string) {
			mu.Lock()
			updates = append(updates, text)
			mu.Unlock()
		})
		if err != nil {
			t.Error("ran into unexpected error when instantiating test PollableFile: %w", err)
		}

		testString := []byte("string1")

		done := make(chan struct{})
		go pf.Start(done)

		// wait a little bit, then write the first string to the monitored file
		time.Sleep(testInterval / 5)
		os.WriteFile(pf.File, testString, 0644)

		// wait long enough for the changes to get picked up, then stop polling
		time.Sleep(testInterval)
		close(done)

		// allow goroutine to exit
		time.Sleep(5 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		if len(updates) != 1 {
			t.Errorf("expected 1 update, got %d: %v", len(updates), updates)
		} else if updates[0] != string(testString) {
			t.Errorf("expected update %q, got %q", string(testString), updates[0])
		}
	})

	t.Run("MultipleUpdates", func(t *testing.T) {
		testInterval := 50 * time.Millisecond

		var mu sync.Mutex
		var updates []string
		pf, err := NewPollableFile(testInterval, func(text string) {
			mu.Lock()
			updates = append(updates, text)
			mu.Unlock()
		})
		if err != nil {
			t.Error("ran into unexpected error when instantiating test PollableFile: %w", err)
		}

		testString1 := []byte("string1")
		testString2 := []byte("anotherstring")
		testString3 := []byte("notastring")

		done := make(chan struct{})
		go pf.Start(done)

		// wait a little bit, then write the first string to the monitored file
		time.Sleep(testInterval / 5)
		os.WriteFile(pf.File, testString1, 0644)

		// wait long enough for the changes to get picked up, then write the second string
		time.Sleep(testInterval)
		os.WriteFile(pf.File, testString2, 0644)

		// wait a little, but not long enough for the changes to get picked up, then write the third string
		time.Sleep(2 * testInterval / 5)
		os.WriteFile(pf.File, testString3, 0644)

		// wait long enough for the changes to get picked up, then stop polling
		time.Sleep(testInterval)
		close(done)

		// allow goroutine to exit
		time.Sleep(5 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		if len(updates) != 2 {
			t.Errorf("expected 2 update, got %d: %v", len(updates), updates)
		} else if updates[0] != string(testString1) {
			t.Errorf("expected update %q, got %q", string(testString1), updates[0])
		} else if updates[1] != string(testString3) {
			t.Errorf("expected update %q, got %q", string(testString3), updates[1])
		}
	})
}
