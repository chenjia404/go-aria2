package manager

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/session"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

type stubDriver struct {
	added []*task.AddTaskInput
	tasks map[string]*task.Task
}

type recordingStore struct {
	saved []*task.Task
}

func (s *recordingStore) Load(ctx context.Context) ([]*task.Task, error) {
	_ = ctx
	if s == nil || len(s.saved) == 0 {
		return nil, nil
	}
	out := make([]*task.Task, 0, len(s.saved))
	for _, item := range s.saved {
		out = append(out, item.Clone())
	}
	return out, nil
}

func (s *recordingStore) Save(ctx context.Context, tasks []*task.Task) error {
	_ = ctx
	s.saved = make([]*task.Task, 0, len(tasks))
	for _, item := range tasks {
		s.saved = append(s.saved, item.Clone())
	}
	return nil
}

type refreshingSessionDriver struct {
	tasks map[string]*task.Task
}

func newRefreshingSessionDriver() *refreshingSessionDriver {
	return &refreshingSessionDriver{tasks: make(map[string]*task.Task)}
}

func (d *refreshingSessionDriver) Name() string { return "bt" }

func (d *refreshingSessionDriver) CanHandle(input task.AddTaskInput) bool { return true }

func (d *refreshingSessionDriver) Add(ctx context.Context, input task.AddTaskInput) (*task.Task, error) {
	_ = ctx
	item := &task.Task{
		ID:       "bt-task",
		GID:      "bt-gid",
		Protocol: task.ProtocolBT,
		Name:     "sample.torrent",
		Status:   task.StatusPaused,
		SaveDir:  input.SaveDir,
	}
	d.tasks[item.ID] = item.Clone()
	return item.Clone(), nil
}

func (d *refreshingSessionDriver) Start(ctx context.Context, taskID string) error {
	_ = ctx
	if item := d.tasks[taskID]; item != nil {
		item.Status = task.StatusActive
	}
	return nil
}

func (d *refreshingSessionDriver) Pause(ctx context.Context, taskID string, force bool) error {
	_ = ctx
	_ = force
	if item := d.tasks[taskID]; item != nil {
		item.Status = task.StatusPaused
	}
	return nil
}

func (d *refreshingSessionDriver) Remove(ctx context.Context, taskID string, force bool) error {
	_ = ctx
	_ = force
	delete(d.tasks, taskID)
	return nil
}

func (d *refreshingSessionDriver) TellStatus(ctx context.Context, taskID string) (*task.Task, error) {
	_ = ctx
	item := d.tasks[taskID]
	if item == nil {
		return nil, ErrTaskNotFound
	}
	return item.Clone(), nil
}

func (d *refreshingSessionDriver) GetFiles(ctx context.Context, taskID string) ([]task.File, error) {
	_ = ctx
	_ = taskID
	return nil, nil
}

func (d *refreshingSessionDriver) ChangeOption(ctx context.Context, taskID string, opts map[string]string) error {
	_ = ctx
	_ = taskID
	_ = opts
	return nil
}

var _ session.Store = (*recordingStore)(nil)

func newStubDriver() *stubDriver {
	return &stubDriver{tasks: make(map[string]*task.Task)}
}

func (d *stubDriver) Name() string { return "http" }

func (d *stubDriver) CanHandle(input task.AddTaskInput) bool { return true }

func (d *stubDriver) Add(ctx context.Context, input task.AddTaskInput) (*task.Task, error) {
	_ = ctx

	cloned := input
	cloned.Options = cloneOptions(input.Options)
	cloned.Meta = cloneOptions(input.Meta)
	d.added = append(d.added, &cloned)

	id := fmt.Sprintf("task-%d", len(d.added))
	item := &task.Task{
		ID:       id,
		GID:      fmt.Sprintf("gid-%d", len(d.added)),
		Protocol: task.ProtocolHTTP,
		Name:     "stub",
		Status:   task.StatusWaiting,
		SaveDir:  input.SaveDir,
		Options:  cloneOptions(input.Options),
	}
	d.tasks[id] = item.Clone()
	return item.Clone(), nil
}

func (d *stubDriver) Start(ctx context.Context, taskID string) error {
	_ = ctx
	if item := d.tasks[taskID]; item != nil {
		item.Status = task.StatusActive
	}
	return nil
}

func (d *stubDriver) Pause(ctx context.Context, taskID string, force bool) error {
	_ = ctx
	_ = force
	if item := d.tasks[taskID]; item != nil {
		item.Status = task.StatusPaused
	}
	return nil
}

func (d *stubDriver) Remove(ctx context.Context, taskID string, force bool) error {
	_ = ctx
	_ = force
	if item := d.tasks[taskID]; item != nil {
		item.Status = task.StatusRemoved
	}
	return nil
}

func (d *stubDriver) PurgeLocalState(taskID string) {
	delete(d.tasks, taskID)
}

func (d *stubDriver) TellStatus(ctx context.Context, taskID string) (*task.Task, error) {
	_ = ctx
	item := d.tasks[taskID]
	if item == nil {
		return nil, ErrTaskNotFound
	}
	return item.Clone(), nil
}

func (d *stubDriver) GetFiles(ctx context.Context, taskID string) ([]task.File, error) {
	_ = ctx
	_ = taskID
	return nil, nil
}

func (d *stubDriver) ChangeOption(ctx context.Context, taskID string, opts map[string]string) error {
	_ = ctx
	item := d.tasks[taskID]
	if item == nil {
		return ErrTaskNotFound
	}
	if item.Options == nil {
		item.Options = map[string]string{}
	}
	for key, value := range opts {
		item.Options[key] = value
	}
	return nil
}

func TestManagerAppliesGlobalOptions(t *testing.T) {
	t.Parallel()

	driver := newStubDriver()
	mgr := New(Options{
		DefaultDir:    "./default",
		StartPaused:   true,
		GlobalOptions: map[string]string{"dir": "./global", "pause": "true", "http-user-agent": "global-agent"},
	})
	mgr.RegisterDriver(driver)

	created, err := mgr.Add(context.Background(), task.AddTaskInput{
		URI: "http://example.com/file",
		Options: map[string]string{
			"http-user-agent": "task-agent",
		},
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if created.SaveDir != "./global" {
		t.Fatalf("expected global dir, got %+v", created)
	}
	if created.Status != task.StatusPaused {
		t.Fatalf("expected paused status, got %+v", created)
	}
	if len(driver.added) != 1 {
		t.Fatalf("expected one add call, got %d", len(driver.added))
	}
	first := driver.added[0]
	if first.SaveDir != "./global" {
		t.Fatalf("driver saw unexpected save dir: %+v", first)
	}
	if first.Options["dir"] != "./global" {
		t.Fatalf("driver saw unexpected merged dir: %+v", first.Options)
	}
	if first.Options["http-user-agent"] != "task-agent" {
		t.Fatalf("task option should override global option: %+v", first.Options)
	}
	if first.Options["pause"] != "true" {
		t.Fatalf("expected pause to come from global options: %+v", first.Options)
	}

	updated := mgr.ChangeGlobalOption(map[string]string{
		"dir":                      "./runtime",
		"pause":                    "false",
		"max-concurrent-downloads": "3",
	})
	if updated["dir"] != "./runtime" || updated["pause"] != "false" {
		t.Fatalf("unexpected updated global options: %+v", updated)
	}

	created2, err := mgr.Add(context.Background(), task.AddTaskInput{
		URI: "http://example.com/second",
	})
	if err != nil {
		t.Fatalf("second Add returned error: %v", err)
	}
	if created2.SaveDir != "./runtime" {
		t.Fatalf("expected updated dir, got %+v", created2)
	}
	if created2.Status != task.StatusActive {
		t.Fatalf("expected active status after pause=false, got %+v", created2)
	}
	if len(driver.added) != 2 {
		t.Fatalf("expected two add calls, got %d", len(driver.added))
	}
	second := driver.added[1]
	if second.SaveDir != "./runtime" {
		t.Fatalf("driver saw unexpected updated save dir: %+v", second)
	}
	if second.Options["pause"] != "false" {
		t.Fatalf("expected updated pause flag in merged options: %+v", second.Options)
	}
}

func TestRemovePurgesTaskFromManager(t *testing.T) {
	t.Parallel()

	driver := newStubDriver()
	mgr := New(Options{DefaultDir: "./default"})
	mgr.RegisterDriver(driver)

	created, err := mgr.Add(context.Background(), task.AddTaskInput{
		URI: "http://example.com/file",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	gid := created.GID
	if _, err := mgr.Remove(context.Background(), gid, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if mgr.GetByGID(gid) != nil {
		t.Fatal("expected task removed from manager")
	}
	_, err = mgr.TellStatus(context.Background(), gid)
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("TellStatus after remove: want ErrTaskNotFound, got %v", err)
	}
}

func TestRemoveConcurrentSameGID(t *testing.T) {
	t.Parallel()

	driver := newStubDriver()
	mgr := New(Options{DefaultDir: "./default"})
	mgr.RegisterDriver(driver)

	created, err := mgr.Add(context.Background(), task.AddTaskInput{
		URI: "http://example.com/file",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	gid := created.GID

	var wg sync.WaitGroup
	wg.Add(2)
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			<-start
			_, errs[i] = mgr.Remove(context.Background(), gid, false)
			wg.Done()
		}()
	}
	close(start)
	wg.Wait()

	var ok, notFound int
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, ErrTaskNotFound):
			notFound++
		default:
			t.Fatalf("unexpected error: %v", e)
		}
	}
	if ok != 1 || notFound != 1 {
		t.Fatalf("want one success and one ErrTaskNotFound, got ok=%d notFound=%d errs=%v", ok, notFound, errs)
	}
	if mgr.GetByGID(gid) != nil {
		t.Fatal("task should be removed")
	}
}

func TestSaveSessionRefreshesDriverStateBeforePersist(t *testing.T) {
	t.Parallel()

	store := &recordingStore{}
	driver := newRefreshingSessionDriver()
	mgr := New(Options{
		DefaultDir: "./downloads",
		Store:      store,
	})
	mgr.RegisterDriver(driver)

	created, err := mgr.Add(context.Background(), task.AddTaskInput{
		URI:     "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567",
		SaveDir: "./downloads",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	driver.tasks[created.ID] = &task.Task{
		ID:              created.ID,
		GID:             created.GID,
		Protocol:        task.ProtocolBT,
		Name:            created.Name,
		Status:          task.StatusPaused,
		SaveDir:         created.SaveDir,
		CompletedLength: 4096,
		VerifiedLength:  4096,
		Files: []task.File{{
			Index:           1,
			Path:            "./downloads/sample.bin",
			Length:          8192,
			CompletedLength: 4096,
			Selected:        true,
		}},
	}

	if err := mgr.SaveSession(context.Background()); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("expected one persisted task, got %d", len(store.saved))
	}
	saved := store.saved[0]
	if saved.CompletedLength != 4096 || saved.VerifiedLength != 4096 {
		t.Fatalf("persisted stale progress: %+v", saved)
	}
	if len(saved.Files) != 1 || saved.Files[0].CompletedLength != 4096 {
		t.Fatalf("persisted stale file progress: %+v", saved.Files)
	}
	if saved.Status != task.StatusPaused {
		t.Fatalf("unexpected persisted status: %+v", saved)
	}
}
