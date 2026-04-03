package aria2

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

type rpcStubDriver struct {
	tasks map[string]*task.Task
}

func newRPCStubDriver() *rpcStubDriver {
	return &rpcStubDriver{tasks: make(map[string]*task.Task)}
}

func (d *rpcStubDriver) Name() string { return "http" }

func (d *rpcStubDriver) CanHandle(input task.AddTaskInput) bool { return true }

func (d *rpcStubDriver) Add(ctx context.Context, input task.AddTaskInput) (*task.Task, error) {
	_ = ctx
	item := &task.Task{
		ID:       "task-1",
		GID:      "gid-1",
		Protocol: task.ProtocolHTTP,
		Name:     input.Name,
		Status:   task.StatusWaiting,
		SaveDir:  input.SaveDir,
		Files: []task.File{{
			Index:    0,
			Path:     filepath.Join(input.SaveDir, "download.bin"),
			Selected: true,
			URIs:     append([]string(nil), input.URIs...),
		}},
		Options: cloneOptionMap(input.Options),
	}
	d.tasks[item.ID] = item.Clone()
	return item.Clone(), nil
}

func (d *rpcStubDriver) Start(ctx context.Context, taskID string) error {
	_ = ctx
	if item := d.tasks[taskID]; item != nil {
		item.Status = task.StatusActive
	}
	return nil
}

func (d *rpcStubDriver) Pause(ctx context.Context, taskID string, force bool) error {
	_ = ctx
	_ = force
	if item := d.tasks[taskID]; item != nil {
		item.Status = task.StatusPaused
	}
	return nil
}

func (d *rpcStubDriver) Remove(ctx context.Context, taskID string, force bool) error {
	_ = ctx
	_ = force
	if item := d.tasks[taskID]; item != nil {
		item.Status = task.StatusRemoved
	}
	return nil
}

func (d *rpcStubDriver) PurgeLocalState(taskID string) {
	delete(d.tasks, taskID)
}

func (d *rpcStubDriver) TellStatus(ctx context.Context, taskID string) (*task.Task, error) {
	_ = ctx
	item := d.tasks[taskID]
	if item == nil {
		return nil, manager.ErrTaskNotFound
	}
	return item.Clone(), nil
}

func (d *rpcStubDriver) GetFiles(ctx context.Context, taskID string) ([]task.File, error) {
	_ = ctx
	item := d.tasks[taskID]
	if item == nil {
		return nil, manager.ErrTaskNotFound
	}
	return task.CloneFiles(item.Files), nil
}

func (d *rpcStubDriver) GetPeers(ctx context.Context, taskID string) ([]manager.PeerInfo, error) {
	_ = ctx
	if item := d.tasks[taskID]; item == nil {
		return nil, manager.ErrTaskNotFound
	}
	return []manager.PeerInfo{{
		PeerID:        "peer-1",
		IP:            "10.0.0.2",
		Port:          6881,
		Bitfield:      "ff",
		AmChoking:     true,
		PeerChoking:   false,
		DownloadSpeed: 1234,
		UploadSpeed:   56,
		Seeder:        true,
	}}, nil
}

func (d *rpcStubDriver) GetServers(ctx context.Context, taskID string) ([]manager.FileServerInfo, error) {
	_ = ctx
	if item := d.tasks[taskID]; item == nil {
		return nil, manager.ErrTaskNotFound
	}
	return []manager.FileServerInfo{{
		Index: 1,
		Servers: []manager.ServerEntry{{
			URI:           "http://example.com/download.bin",
			CurrentURI:    "http://mirror.example.com/download.bin",
			DownloadSpeed: 4321,
		}},
	}}, nil
}

func (d *rpcStubDriver) ChangeOption(ctx context.Context, taskID string, opts map[string]string) error {
	_ = ctx
	item := d.tasks[taskID]
	if item == nil {
		return manager.ErrTaskNotFound
	}
	if item.Options == nil {
		item.Options = map[string]string{}
	}
	for key, value := range opts {
		item.Options[key] = value
	}
	return nil
}

func TestServiceExposesVersionAndSessionMethods(t *testing.T) {
	t.Parallel()

	service := NewService(manager.New(manager.Options{GlobalOptions: map[string]string{"dir": "./initial"}}), "")

	rawVersion, err := service.Invoke(context.Background(), "aria2.getVersion", nil)
	if err != nil {
		t.Fatalf("getVersion returned error: %v", err)
	}
	version, ok := rawVersion.(map[string]any)
	if !ok {
		t.Fatalf("unexpected version payload: %#v", rawVersion)
	}
	if version["version"] == "" {
		t.Fatalf("missing version field: %#v", version)
	}

	rawMethods, err := service.Invoke(context.Background(), "system.listMethods", nil)
	if err != nil {
		t.Fatalf("listMethods returned error: %v", err)
	}
	methods, ok := rawMethods.([]string)
	if !ok {
		t.Fatalf("unexpected methods payload: %#v", rawMethods)
	}

	required := map[string]bool{
		"aria2.getVersion":           false,
		"aria2.getSessionInfo":       false,
		"aria2.getGlobalOption":      false,
		"aria2.changeGlobalOption":   false,
		"aria2.getPeers":             false,
		"aria2.getServers":           false,
		"aria2.pauseAll":             false,
		"aria2.unpauseAll":           false,
		"aria2.removeDownloadResult": false,
	}
	for _, method := range methods {
		if _, ok := required[method]; ok {
			required[method] = true
		}
	}
	for method, found := range required {
		if !found {
			t.Fatalf("expected %s in method list: %#v", method, methods)
		}
	}

	rawGlobal, err := service.Invoke(context.Background(), "aria2.getGlobalOption", nil)
	if err != nil {
		t.Fatalf("getGlobalOption returned error: %v", err)
	}
	global, ok := rawGlobal.(map[string]string)
	if !ok {
		t.Fatalf("unexpected global option payload: %#v", rawGlobal)
	}
	if global["dir"] != "./initial" {
		t.Fatalf("expected dir in global options: %#v", global)
	}

	changed, err := service.Invoke(context.Background(), "aria2.changeGlobalOption", []any{
		map[string]any{"dir": "./tmp-method-test"},
	})
	if err != nil {
		t.Fatalf("changeGlobalOption returned error: %v", err)
	}
	changedMap, ok := changed.(map[string]string)
	if !ok {
		t.Fatalf("unexpected changed payload: %#v", changed)
	}
	if changedMap["dir"] != "./tmp-method-test" {
		t.Fatalf("expected updated dir, got %#v", changedMap)
	}
}

func TestOptionAndURIHelpers(t *testing.T) {
	t.Parallel()

	item := &task.Task{
		Name:    "sample.bin",
		SaveDir: "./downloads",
		Status:  task.StatusPaused,
		Options: map[string]string{"http-user-agent": "ua"},
		Files: []task.File{
			{URIs: []string{"http://a.example/file", "http://b.example/file"}},
			{URIs: []string{"http://a.example/file", "magnet:?xt=urn:btih:abc"}},
		},
	}

	opts := toOptionResponse(item)
	if opts["dir"] != "./downloads" || opts["pause"] != "true" || opts["out"] != "sample.bin" {
		t.Fatalf("unexpected option mapping: %#v", opts)
	}
	uris := toURIsResponse(item.Files)
	if len(uris) != 3 {
		t.Fatalf("unexpected uri mapping: %#v", uris)
	}
}

func TestBulkCommandsAndDownloadResultRemoval(t *testing.T) {
	t.Parallel()

	saveDir := t.TempDir()
	filePath := filepath.Join(saveDir, "download.bin")
	if err := os.WriteFile(filePath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	driver := newRPCStubDriver()
	service := NewService(manager.New(manager.Options{GlobalOptions: map[string]string{"dir": saveDir}}), "")
	service.manager.RegisterDriver(driver)

	rawGID, err := service.Invoke(context.Background(), "aria2.addUri", []any{
		[]any{"http://example.com/download.bin"},
		map[string]any{"dir": saveDir},
	})
	if err != nil {
		t.Fatalf("addUri returned error: %v", err)
	}
	gid, ok := rawGID.(string)
	if !ok || gid == "" {
		t.Fatalf("unexpected gid payload: %#v", rawGID)
	}

	if _, err := service.Invoke(context.Background(), "aria2.pauseAll", nil); err != nil {
		t.Fatalf("pauseAll returned error: %v", err)
	}
	stopped, err := service.Invoke(context.Background(), "aria2.tellStatus", []any{gid})
	if err != nil {
		t.Fatalf("tellStatus after pauseAll returned error: %v", err)
	}
	stoppedMap, ok := stopped.(map[string]any)
	if !ok || stoppedMap["status"] != "paused" {
		t.Fatalf("expected paused status after pauseAll, got %#v", stopped)
	}
	if _, err := service.Invoke(context.Background(), "aria2.unpauseAll", nil); err != nil {
		t.Fatalf("unpauseAll returned error: %v", err)
	}
	resumed, err := service.Invoke(context.Background(), "aria2.tellStatus", []any{gid})
	if err != nil {
		t.Fatalf("tellStatus after unpauseAll returned error: %v", err)
	}
	resumedMap, ok := resumed.(map[string]any)
	if !ok || resumedMap["status"] != "active" {
		t.Fatalf("expected active status after unpauseAll, got %#v", resumed)
	}
	if _, err := service.Invoke(context.Background(), "aria2.removeDownloadResult", []any{gid}); err != nil {
		t.Fatalf("removeDownloadResult returned error: %v", err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected download result removed, stat err=%v", err)
	}
}

func TestPeersAndServersMethods(t *testing.T) {
	t.Parallel()

	driver := newRPCStubDriver()
	service := NewService(manager.New(manager.Options{GlobalOptions: map[string]string{"dir": t.TempDir()}}), "")
	service.manager.RegisterDriver(driver)

	rawGID, err := service.Invoke(context.Background(), "aria2.addUri", []any{
		[]any{"http://example.com/download.bin"},
		map[string]any{"dir": t.TempDir()},
	})
	if err != nil {
		t.Fatalf("addUri returned error: %v", err)
	}
	gid, ok := rawGID.(string)
	if !ok || gid == "" {
		t.Fatalf("unexpected gid payload: %#v", rawGID)
	}

	rawPeers, err := service.Invoke(context.Background(), "aria2.getPeers", []any{gid})
	if err != nil {
		t.Fatalf("getPeers returned error: %v", err)
	}
	peers, ok := rawPeers.([]map[string]any)
	if !ok || len(peers) != 1 {
		t.Fatalf("unexpected peers payload: %#v", rawPeers)
	}
	if peers[0]["ip"] != "10.0.0.2" || peers[0]["seeder"] != "true" {
		t.Fatalf("unexpected peer mapping: %#v", peers[0])
	}

	rawServers, err := service.Invoke(context.Background(), "aria2.getServers", []any{gid})
	if err != nil {
		t.Fatalf("getServers returned error: %v", err)
	}
	servers, ok := rawServers.([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("unexpected servers payload: %#v", rawServers)
	}
	serversList, ok := servers[0]["servers"].([]map[string]any)
	if !ok || len(serversList) != 1 {
		t.Fatalf("unexpected nested servers mapping: %#v", servers[0])
	}
	if serversList[0]["currentUri"] != "http://mirror.example.com/download.bin" {
		t.Fatalf("unexpected server mapping: %#v", serversList[0])
	}
}
