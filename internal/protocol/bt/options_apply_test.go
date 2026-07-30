package bt

import (
	"bytes"
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestChangeOption_BTMaxPeers(t *testing.T) {
	t.Parallel()

	saveDir := mustTempDir(t)
	defer removeDirEventually(t, saveDir)
	payload := buildTestTorrentPayload(t, saveDir, "peers.bin", bytes.Repeat([]byte("p"), 4096))

	dataDir := mustTempDir(t)
	defer removeDirEventually(t, dataDir)
	driver, err := New(Options{DataDir: dataDir, ListenPort: 0, MaxPeers: 30})
	if err != nil {
		t.Fatalf("new bt driver: %v", err)
	}
	defer driver.Close()

	item, err := driver.Add(context.Background(), task.AddTaskInput{
		Torrent: payload,
		SaveDir: saveDir,
		Options: map[string]string{"pause": "true"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	driver.mu.Lock()
	st := driver.tasks[item.ID]
	tor := st.torrent
	driver.mu.Unlock()
	if tor == nil {
		t.Fatal("torrent not created")
	}

	if err := driver.ChangeOption(context.Background(), item.ID, map[string]string{"bt-max-peers": "42"}); err != nil {
		t.Fatalf("ChangeOption: %v", err)
	}
	oldMax := tor.SetMaxEstablishedConns(99)
	if oldMax != 42 {
		t.Fatalf("expected max peers 42 after changeOption, got %d", oldMax)
	}
}

func TestApplyBTMaxPeers_UsesTaskOption(t *testing.T) {
	t.Parallel()

	saveDir := mustTempDir(t)
	defer removeDirEventually(t, saveDir)
	payload := buildTestTorrentPayload(t, saveDir, "opt.bin", bytes.Repeat([]byte("o"), 4096))

	dataDir := mustTempDir(t)
	defer removeDirEventually(t, dataDir)
	driver, err := New(Options{DataDir: dataDir, ListenPort: 0, MaxPeers: 10})
	if err != nil {
		t.Fatalf("new bt driver: %v", err)
	}
	defer driver.Close()

	item, err := driver.Add(context.Background(), task.AddTaskInput{
		Torrent: payload,
		SaveDir: saveDir,
		Options: map[string]string{"pause": "true", "bt-max-peers": "25"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	driver.mu.Lock()
	tor := driver.tasks[item.ID].torrent
	driver.mu.Unlock()

	oldMax := tor.SetMaxEstablishedConns(99)
	if oldMax != 25 {
		t.Fatalf("expected max peers 25 on add, got %d", oldMax)
	}
}
