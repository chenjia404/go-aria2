package bt

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	torrentlib "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestLoadSessionTasks_RebuildsProgressForRegularTorrentTasks(t *testing.T) {
	t.Parallel()

	saveDir := mustTempDir(t)
	defer removeDirEventually(t, saveDir)
	payload := buildTestTorrentPayload(t, saveDir, "movie.bin", bytes.Repeat([]byte("x"), 4096))

	dataDir := mustTempDir(t)
	defer removeDirEventually(t, dataDir)
	driver, err := New(Options{DataDir: dataDir, ListenPort: 0})
	if err != nil {
		t.Fatalf("new bt driver: %v", err)
	}
	defer driver.Close()

	added, err := parseAddInput(context.Background(), task.AddTaskInput{Torrent: payload})
	if err != nil {
		t.Fatalf("parseAddInput: %v", err)
	}

	called := make(chan *task.Task, 1)
	driver.rebuildProgress = func(item *task.Task, _ *torrentlib.Torrent) error {
		called <- item.Clone()
		return nil
	}

	saved := &task.Task{
		ID:       "bt-task-1",
		GID:      "gid-1",
		Protocol: task.ProtocolBT,
		Name:     "movie.bin",
		Status:   task.StatusPaused,
		SaveDir:  saveDir,
		Meta:     buildSourceMeta(nil, added.Source),
	}
	if err := driver.LoadSessionTasks(context.Background(), []*task.Task{saved}, nil); err != nil {
		t.Fatalf("load session tasks: %v", err)
	}

	select {
	case rebuilt := <-called:
		if rebuilt.SaveDir != saveDir {
			t.Fatalf("rebuild received wrong save dir: %+v", rebuilt)
		}
		if rebuilt.ID != saved.ID {
			t.Fatalf("rebuild received wrong task: %+v", rebuilt)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for rebuilt progress")
	}
}

func TestLoadSessionTasks_DelaysActiveResumeUntilProgressRebuildFinishes(t *testing.T) {
	t.Parallel()

	saveDir := mustTempDir(t)
	defer removeDirEventually(t, saveDir)
	payload := buildTestTorrentPayload(t, saveDir, "episode.bin", bytes.Repeat([]byte("y"), 4096))

	dataDir := mustTempDir(t)
	defer removeDirEventually(t, dataDir)
	driver, err := New(Options{DataDir: dataDir, ListenPort: 0})
	if err != nil {
		t.Fatalf("new bt driver: %v", err)
	}
	defer driver.Close()

	added, err := parseAddInput(context.Background(), task.AddTaskInput{Torrent: payload})
	if err != nil {
		t.Fatalf("parseAddInput: %v", err)
	}

	blocked := make(chan struct{})
	driver.rebuildProgress = func(item *task.Task, _ *torrentlib.Torrent) error {
		<-blocked
		return nil
	}

	saved := &task.Task{
		ID:       "bt-task-2",
		GID:      "gid-2",
		Protocol: task.ProtocolBT,
		Name:     "episode.bin",
		Status:   task.StatusActive,
		SaveDir:  saveDir,
		Meta:     buildSourceMeta(nil, added.Source),
	}
	if err := driver.LoadSessionTasks(context.Background(), []*task.Task{saved}, nil); err != nil {
		t.Fatalf("load session tasks: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	before, err := driver.TellStatus(context.Background(), saved.ID)
	if err != nil {
		t.Fatalf("tell status before resume: %v", err)
	}
	if before.Status == task.StatusActive {
		t.Fatalf("task resumed before progress rebuild finished: %+v", before)
	}

	close(blocked)

	deadline := time.Now().Add(5 * time.Second)
	for {
		after, err := driver.TellStatus(context.Background(), saved.ID)
		if err != nil {
			t.Fatalf("tell status after resume: %v", err)
		}
		if after.Status == task.StatusActive || after.Status == task.StatusComplete {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task did not resume after rebuild: %+v", after)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestLoadSessionTasks_PausedTaskStaysPausedAfterProgressRebuild(t *testing.T) {
	t.Parallel()

	saveDir := mustTempDir(t)
	defer removeDirEventually(t, saveDir)
	payloadData := bytes.Repeat([]byte("z"), 4096)
	payload := buildTestTorrentPayload(t, saveDir, "paused.bin", payloadData)

	dataDir := mustTempDir(t)
	defer removeDirEventually(t, dataDir)
	driver, err := New(Options{DataDir: dataDir, ListenPort: 0})
	if err != nil {
		t.Fatalf("new bt driver: %v", err)
	}
	defer driver.Close()

	added, err := parseAddInput(context.Background(), task.AddTaskInput{Torrent: payload})
	if err != nil {
		t.Fatalf("parseAddInput: %v", err)
	}

	called := make(chan struct{}, 1)
	driver.rebuildProgress = func(item *task.Task, _ *torrentlib.Torrent) error {
		called <- struct{}{}
		return nil
	}

	saved := &task.Task{
		ID:       "bt-task-3",
		GID:      "gid-3",
		Protocol: task.ProtocolBT,
		Name:     "paused.bin",
		Status:   task.StatusPaused,
		SaveDir:  saveDir,
		Meta:     buildSourceMeta(nil, added.Source),
	}
	if err := driver.LoadSessionTasks(context.Background(), []*task.Task{saved}, nil); err != nil {
		t.Fatalf("load session tasks: %v", err)
	}

	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for rebuilt progress")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		current, err := driver.TellStatus(context.Background(), saved.ID)
		if err != nil {
			t.Fatalf("tell status: %v", err)
		}
		if current.Status != task.StatusWaiting {
			if current.Status != task.StatusPaused {
				t.Fatalf("paused task unexpectedly changed state: %+v", current)
			}
			if current.CompletedLength != int64(len(payloadData)) {
				t.Fatalf("paused task lost rebuilt progress: %+v", current)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("paused task did not settle back to paused: %+v", current)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRebuildBTProgress_SingleFileTorrentUsesCorrectPath(t *testing.T) {
	t.Parallel()

	saveDir := mustTempDir(t)
	defer removeDirEventually(t, saveDir)
	payloadData := bytes.Repeat([]byte("a"), 64*1024)
	torrentName := "ubuntu.iso"
	payload := buildTestTorrentPayload(t, saveDir, torrentName, payloadData)

	driver, err := New(Options{DataDir: mustTempDir(t), ListenPort: 0})
	if err != nil {
		t.Fatalf("new bt driver: %v", err)
	}
	defer driver.Close()

	created, err := driver.Add(context.Background(), task.AddTaskInput{
		Torrent: payload,
		SaveDir: saveDir,
	})
	if err != nil {
		t.Fatalf("add torrent: %v", err)
	}

	status, err := driver.TellStatus(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("tell status: %v", err)
	}
	status.Files = []task.File{{
		Index:    1,
		Path:     torrentName,
		Length:   int64(len(payloadData)),
		Selected: true,
	}}
	if err := RebuildBTProgress(status, driver.tasks[created.ID].torrent); err != nil {
		t.Fatalf("rebuild progress: %v", err)
	}
	if status.CompletedLength != int64(len(payloadData)) {
		t.Fatalf("single-file rebuild lost progress: completed=%d want=%d meta=%+v", status.CompletedLength, len(payloadData), status.Meta)
	}
	if status.Meta["bt.completedPieces"] == "0" {
		t.Fatalf("single-file rebuild reported zero pieces: %+v", status.Meta)
	}
}

func buildTestTorrentPayload(t *testing.T, dir, name string, data []byte) []byte {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	mi := metainfo.MetaInfo{}
	mi.SetDefaults()
	info := metainfo.Info{PieceLength: 16 * 1024}
	if err := info.BuildFromFilePath(path); err != nil {
		t.Fatalf("build torrent info: %v", err)
	}
	info.Name = name

	rawInfo, err := bencode.Marshal(info)
	if err != nil {
		t.Fatalf("marshal info: %v", err)
	}
	mi.InfoBytes = rawInfo

	var buf bytes.Buffer
	if err := mi.Write(&buf); err != nil {
		t.Fatalf("write metainfo: %v", err)
	}
	return buf.Bytes()
}

func mustTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "go-aria2-bt-test-*")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	return dir
}

func removeDirEventually(t *testing.T, dir string) {
	t.Helper()
	for i := 0; i < 20; i++ {
		if err := os.RemoveAll(dir); err == nil || os.IsNotExist(err) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = os.RemoveAll(dir)
}
