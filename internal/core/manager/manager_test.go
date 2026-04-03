package manager

import (
	"context"
	"fmt"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

type stubDriver struct {
	added []*task.AddTaskInput
	tasks map[string]*task.Task
}

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
