package aria2

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/session"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

type rpcStubDriver struct {
	added []*task.AddTaskInput
	tasks map[string]*task.Task
}

func newRPCStubDriver() *rpcStubDriver {
	return &rpcStubDriver{tasks: make(map[string]*task.Task)}
}

func (d *rpcStubDriver) Name() string { return "http" }

func (d *rpcStubDriver) CanHandle(input task.AddTaskInput) bool { return true }

func (d *rpcStubDriver) Add(ctx context.Context, input task.AddTaskInput) (*task.Task, error) {
	_ = ctx

	cloned := input
	cloned.Options = cloneOptionMap(input.Options)
	cloned.Meta = cloneOptionMap(input.Meta)
	d.added = append(d.added, &cloned)

	id := fmt.Sprintf("task-%d", len(d.tasks)+1)
	name := input.Name
	if name == "" {
		if len(input.Torrent) > 0 {
			name = "test.bin"
		} else {
			name = "download.bin"
		}
	}
	uris := append([]string(nil), input.URIs...)
	if input.URI != "" {
		uris = append([]string{input.URI}, uris...)
	}
	if len(uris) == 0 && len(input.Torrent) == 0 {
		uris = []string{"http://localhost/placeholder"}
	}

	protocol := task.ProtocolHTTP
	if len(input.Torrent) > 0 {
		protocol = task.ProtocolBT
	}

	item := &task.Task{
		ID:       id,
		GID:      fmt.Sprintf("gid-%d", len(d.tasks)+1),
		Protocol: protocol,
		Name:     name,
		Status:   task.StatusWaiting,
		SaveDir:  input.SaveDir,
		Files: []task.File{{
			Index:    1,
			Path:     filepath.Join(input.SaveDir, name),
			Selected: true,
			URIs:     uris,
		}},
		Options: cloneOptionMap(input.Options),
		Meta:    cloneOptionMap(input.Meta),
	}
	if len(input.Torrent) > 0 {
		if item.Meta == nil {
			item.Meta = map[string]string{}
		}
		item.Meta["bt.source.torrentBytes"] = "true"
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
	// HTTP 任务与 aria2 一致：getPeers 返回空数组。
	return []manager.PeerInfo{}, nil
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
	if dir := strings.TrimSpace(opts["dir"]); dir != "" {
		item.SaveDir = dir
	}
	return nil
}

func (d *rpcStubDriver) ChangeURI(ctx context.Context, taskID string, fileIndex int, delURIs, addURIs []string, position int) (int, int, error) {
	_ = ctx
	item := d.tasks[taskID]
	if item == nil || len(item.Files) == 0 {
		return 0, 0, manager.ErrTaskNotFound
	}
	idx := fileIndex - 1
	if idx < 0 || idx >= len(item.Files) {
		return 0, 0, fmt.Errorf("file not found")
	}
	uris := append([]string(nil), item.Files[idx].URIs...)
	delCount := 0
	for _, delURI := range delURIs {
		found := false
		filtered := uris[:0]
		for _, uri := range uris {
			if uri == delURI {
				found = true
				continue
			}
			filtered = append(filtered, uri)
		}
		if found {
			delCount++
		}
		uris = filtered
	}
	addCount := 0
	if len(addURIs) > 0 {
		if position < 0 {
			for _, uri := range addURIs {
				if !isValidURI(uri) {
					continue
				}
				uris = append(uris, uri)
				addCount++
			}
		} else {
			pos := position
			if pos > len(uris) {
				pos = len(uris)
			}
			for _, uri := range addURIs {
				if !isValidURI(uri) {
					continue
				}
				uris = append(append(append([]string(nil), uris[:pos]...), uri), uris[pos:]...)
				addCount++
				pos++
			}
		}
	}
	item.Files[idx].URIs = uris
	return delCount, addCount, nil
}

func TestService_GetVersionEnabledProtocols(t *testing.T) {
	t.Parallel()

	service := NewService(manager.New(manager.Options{}), "")
	raw, err := service.Invoke(context.Background(), "aria2.getVersion", nil)
	if err != nil {
		t.Fatal(err)
	}
	version, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected version payload: %#v", raw)
	}
	enabled, ok := version["enabledProtocols"].([]string)
	if !ok {
		t.Fatalf("enabledProtocols: %#v", version["enabledProtocols"])
	}
	for _, proto := range []string{"ftp", "sftp"} {
		found := false
		for _, item := range enabled {
			if item == proto {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("enabledProtocols missing %q: %#v", proto, enabled)
		}
	}
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
		"aria2.forcePauseAll":        false,
		"aria2.unpauseAll":           false,
		"aria2.changePosition":       false,
		"aria2.changeUri":            false,
		"aria2.removeDownloadResult": false,
		"aria2.purgeDownloadResult":  false,
		"aria2.shutdown":             false,
		"aria2.addMetalink":          false,
		"aria2.ping":                 false,
		"aria2.saveSession":          false,
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
	if opts["dir"] != "./downloads" || opts["out"] != "sample.bin" {
		t.Fatalf("unexpected option mapping: %#v", opts)
	}
	if _, hasPause := opts["pause"]; hasPause {
		t.Fatalf("getOption should not expose pause: %#v", opts)
	}
	uris := toURIsResponse(item.Files, item.Status)
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
	for _, item := range driver.tasks {
		item.Status = task.StatusComplete
		break
	}
	if _, err := service.Invoke(context.Background(), "aria2.removeDownloadResult", []any{gid}); err != nil {
		t.Fatalf("removeDownloadResult returned error: %v", err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected download result removed, stat err=%v", err)
	}
	if _, err := service.Invoke(context.Background(), "aria2.tellStatus", []any{gid}); err == nil {
		t.Fatalf("expected task removed after removeDownloadResult")
	}
}

func TestUnpauseAllResumesPausedTasks(t *testing.T) {
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
	gid := rawGID.(string)

	if _, err := service.Invoke(context.Background(), "aria2.pauseAll", nil); err != nil {
		t.Fatalf("pauseAll returned error: %v", err)
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
}

func TestPurgeDownloadResultClearsStoppedTasks(t *testing.T) {
	t.Parallel()

	driver := newRPCStubDriver()
	mgr := manager.New(manager.Options{GlobalOptions: map[string]string{"dir": t.TempDir()}})
	mgr.RegisterDriver(driver)
	service := NewService(mgr, "")

	rawGID, err := service.Invoke(context.Background(), "aria2.addUri", []any{
		[]any{"http://example.com/download.bin"},
		map[string]any{"dir": t.TempDir()},
	})
	if err != nil {
		t.Fatalf("addUri returned error: %v", err)
	}
	gid := rawGID.(string)
	for _, item := range driver.tasks {
		item.Status = task.StatusComplete
		break
	}

	if _, err := service.Invoke(context.Background(), "aria2.purgeDownloadResult", nil); err != nil {
		t.Fatalf("purgeDownloadResult returned error: %v", err)
	}
	if _, err := service.Invoke(context.Background(), "aria2.tellStatus", []any{gid}); err == nil {
		t.Fatalf("expected stopped task purged from manager")
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
	for _, item := range driver.tasks {
		item.Status = task.StatusActive
	}

	rawPeers, err := service.Invoke(context.Background(), "aria2.getPeers", []any{gid})
	if err != nil {
		t.Fatalf("getPeers returned error: %v", err)
	}
	peers, ok := rawPeers.([]map[string]any)
	if !ok || len(peers) != 0 {
		t.Fatalf("HTTP getPeers should be empty, got %#v", rawPeers)
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

func TestPingAndSaveSession(t *testing.T) {
	t.Parallel()

	sessionPath := filepath.Join(t.TempDir(), "session.json")
	store := session.NewFileStore(sessionPath)
	mgr := manager.New(manager.Options{
		DefaultDir: "./downloads",
		Store:      store,
	})
	mgr.RegisterDriver(newRPCStubDriver())

	service := NewService(mgr, "")
	service.SetSessionPath(sessionPath)

	rawPing, err := service.Invoke(context.Background(), "aria2.ping", nil)
	if err != nil {
		t.Fatalf("ping returned error: %v", err)
	}
	if rawPing != "pong" {
		t.Fatalf("expected pong, got %#v", rawPing)
	}

	_, err = mgr.Add(context.Background(), task.AddTaskInput{
		URI:     "http://example.com/file.bin",
		SaveDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	rawSave, err := service.Invoke(context.Background(), "aria2.saveSession", nil)
	if err != nil {
		t.Fatalf("saveSession returned error: %v", err)
	}
	if rawSave != "OK" {
		t.Fatalf("expected OK, got %#v", rawSave)
	}
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("expected session file at %s: %v", sessionPath, err)
	}

	customPath := filepath.Join(t.TempDir(), "custom-session.json")
	rawSave, err = service.Invoke(context.Background(), "aria2.saveSession", []any{customPath})
	if err != nil {
		t.Fatalf("saveSession with custom path returned error: %v", err)
	}
	if rawSave != "OK" {
		t.Fatalf("expected OK, got %#v", rawSave)
	}
	if _, err := os.Stat(customPath); err != nil {
		t.Fatalf("expected custom session file at %s: %v", customPath, err)
	}
}
