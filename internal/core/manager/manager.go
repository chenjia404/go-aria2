package manager

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/session"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

// btTrackerSyncer 由 BT 驱动实现，用于运行期同步 tracker 列表。
type btTrackerSyncer interface {
	SyncBTTrackerOptions(ctx context.Context, taskID string, opts map[string]string) error
}

func optionKeysAffectBT(opts map[string]string) bool {
	for k := range opts {
		if k == "bt-tracker" || k == "bt-exclude-tracker" {
			return true
		}
	}
	return false
}

// Options 描述任务管理器的启动参数�?
type Options struct {
	DefaultDir    string
	MaxConcurrent int
	StartPaused   bool
	GlobalOptions map[string]string
	Store         session.Store
}

// Manager 负责统一管理不同协议任务的生命周期�?
type Manager struct {
	mu             sync.RWMutex
	tasks          map[string]*task.Task
	drivers        []Driver
	driverByTaskID map[string]Driver
	defaultDir     string
	maxConcurrent  int
	startPaused    bool
	globalOptions  map[string]string
	store          session.Store
	subMu          sync.RWMutex
	nextSubID      int
	subscribers    map[int]chan Event
}

// New 创建任务管理器�?
func New(opts Options) *Manager {
	maxConcurrent := opts.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	return &Manager{
		tasks:          make(map[string]*task.Task),
		driverByTaskID: make(map[string]Driver),
		defaultDir:     opts.DefaultDir,
		maxConcurrent:  maxConcurrent,
		startPaused:    opts.StartPaused,
		globalOptions:  cloneOptions(opts.GlobalOptions),
		store:          opts.Store,
		subscribers:    make(map[int]chan Event),
	}
}

// RegisterDriver 注册统一协议驱动�?
func (m *Manager) RegisterDriver(driver Driver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drivers = append(m.drivers, driver)
}

// Add 创建任务并路由到对应协议驱动�?
func (m *Manager) Add(ctx context.Context, input task.AddTaskInput) (*task.Task, error) {
	driver, err := m.pickDriver(input)
	if err != nil {
		return nil, err
	}

	globalOptions := m.globalOptionsSnapshot()
	perTaskLocal := cloneOptions(input.Options)
	mergedOptions := mergeOptions(globalOptions, perTaskLocal)
	if input.SaveDir == "" {
		if dir := mergedOptions["dir"]; dir != "" {
			input.SaveDir = dir
		} else {
			input.SaveDir = m.defaultDir
		}
	}
	input.Options = mergedOptions
	input.Meta = cloneOptions(input.Meta)

	created, err := driver.Add(ctx, input)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	created = created.Clone()
	if created.ID == "" {
		created.ID = newID()
	}
	if input.GID != "" {
		created.GID = input.GID
	} else if created.GID == "" {
		created.GID = newGID()
	}
	if created.SaveDir == "" {
		created.SaveDir = input.SaveDir
	}
	if created.Status == "" {
		created.Status = task.StatusWaiting
	}
	if created.Options == nil {
		created.Options = cloneOptions(input.Options)
	}
	if created.Meta == nil {
		created.Meta = cloneOptions(input.Meta)
	}
	created.LocalOptions = cloneOptions(perTaskLocal)
	if created.CreatedAt.IsZero() {
		created.CreatedAt = now
	}
	created.UpdatedAt = now

	m.mu.Lock()
	m.tasks[created.ID] = created
	m.driverByTaskID[created.ID] = driver
	m.mu.Unlock()

	addPaused := m.startPaused || parsePauseOption(input.Options)
	if addPaused {
		if err := driver.Pause(ctx, created.ID, false); err != nil {
			return nil, err
		}
		updated, err := driver.TellStatus(ctx, created.ID)
		if err != nil {
			return nil, err
		}
		created = m.storeTask(updated, driver)
	} else if m.shouldStartImmediately() {
		if err := m.startTaskByID(ctx, created.ID); err != nil {
			return nil, err
		}
		if refreshed, err := m.tellStatusByID(ctx, created.ID); err == nil {
			created = refreshed
		}
	}

	if err := m.SaveSession(ctx); err != nil {
		return nil, err
	}
	m.emit(EventTaskAdded, created)

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[created.ID].Clone(), nil
}

// Remove 删除任务�?
func (m *Manager) Remove(ctx context.Context, gid string, force bool) (*task.Task, error) {
	taskID, current, driver, err := m.lookupByGID(gid)
	if err != nil {
		return nil, err
	}

	if err := driver.Remove(ctx, taskID, force); err != nil {
		return nil, err
	}

	updated, err := driver.TellStatus(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if updated.Status == "" {
		updated.Status = task.StatusRemoved
	}
	if updated.GID == "" {
		updated.GID = current.GID
	}
	if updated.ID == "" {
		updated.ID = current.ID
	}

	removed := updated.Clone()
	m.mu.Lock()
	delete(m.tasks, taskID)
	delete(m.driverByTaskID, taskID)
	m.mu.Unlock()
	if p, ok := driver.(LocalStatePurger); ok {
		p.PurgeLocalState(taskID)
	}
	if err := m.SaveSession(ctx); err != nil {
		return nil, err
	}
	m.emit(EventTaskRemoved, removed)
	if err := m.fillSlots(ctx); err != nil {
		return nil, err
	}
	return removed.Clone(), nil
}

// Pause 暂停任务�?
func (m *Manager) Pause(ctx context.Context, gid string, force bool) (*task.Task, error) {
	taskID, _, driver, err := m.lookupByGID(gid)
	if err != nil {
		return nil, err
	}
	if err := driver.Pause(ctx, taskID, force); err != nil {
		return nil, err
	}

	updated, err := driver.TellStatus(ctx, taskID)
	if err != nil {
		return nil, err
	}
	updated = m.storeTask(updated, driver)
	if err := m.SaveSession(ctx); err != nil {
		return nil, err
	}
	m.emit(EventTaskUpdated, updated)
	if err := m.fillSlots(ctx); err != nil {
		return nil, err
	}
	return updated.Clone(), nil
}

// PauseAll 暂停所有可暂停的任务�?
func (m *Manager) PauseAll(ctx context.Context) error {
	taskIDs := m.snapshotTaskIDsByStatus(task.StatusActive, task.StatusWaiting)
	for _, taskID := range taskIDs {
		updated, err := m.pauseTaskByID(ctx, taskID, false)
		if err != nil && !errors.Is(err, ErrTaskNotFound) {
			return err
		}
		if updated != nil {
			m.emit(EventTaskUpdated, updated)
		}
	}
	return m.SaveSession(ctx)
}

// Unpause 恢复已暂停任务�?
func (m *Manager) Unpause(ctx context.Context, gid string) (*task.Task, error) {
	taskID, _, _, err := m.lookupByGID(gid)
	if err != nil {
		return nil, err
	}
	if err := m.startTaskByID(ctx, taskID); err != nil {
		return nil, err
	}

	status, err := m.tellStatusByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if err := m.SaveSession(ctx); err != nil {
		return nil, err
	}
	m.emit(EventTaskUpdated, status)
	return status.Clone(), nil
}

// UnpauseAll 恢复所有暂停或等待中的任务�?
func (m *Manager) UnpauseAll(ctx context.Context) error {
	taskIDs := m.snapshotTaskIDsByStatus(task.StatusPaused, task.StatusWaiting)
	for _, taskID := range taskIDs {
		updated, err := m.unpauseTaskByID(ctx, taskID)
		if err != nil && !errors.Is(err, ErrTaskNotFound) {
			return err
		}
		if updated != nil {
			m.emit(EventTaskUpdated, updated)
		}
	}
	return m.SaveSession(ctx)
}

// RemoveDownloadResult 删除任务对应的下载结果文件，不移除任务本身�?
func (m *Manager) RemoveDownloadResult(ctx context.Context, gid string) error {
	_, current, _, err := m.lookupByGID(gid)
	if err != nil {
		return err
	}

	for _, file := range current.Files {
		if file.Path == "" {
			continue
		}
		_ = os.Remove(file.Path)
	}

	m.emit(EventTaskUpdated, m.GetByGID(gid))
	return nil
}

// TellStatus 返回单个任务状态�?
func (m *Manager) TellStatus(ctx context.Context, gid string) (*task.Task, error) {
	taskID, _, _, err := m.lookupByGID(gid)
	if err != nil {
		return nil, err
	}
	return m.tellStatusByID(ctx, taskID)
}

// TellActive 返回当前 active 任务�?
func (m *Manager) TellActive(ctx context.Context) ([]*task.Task, error) {
	return m.listByStatuses(ctx, task.StatusActive)
}

// TellWaiting 返回 waiting �?paused 任务列表，支持分页�?
func (m *Manager) TellWaiting(ctx context.Context, offset, limit int) ([]*task.Task, error) {
	return m.paginateStatus(ctx, offset, limit, task.StatusWaiting, task.StatusPaused)
}

// TellStopped 返回 stopped 任务列表，支持分页�?
func (m *Manager) TellStopped(ctx context.Context, offset, limit int) ([]*task.Task, error) {
	return m.paginateStatus(ctx, offset, limit, task.StatusComplete, task.StatusError, task.StatusRemoved)
}

// GetFiles 返回任务的文件列表�?
func (m *Manager) GetFiles(ctx context.Context, gid string) ([]task.File, error) {
	taskID, current, driver, err := m.lookupByGID(gid)
	if err != nil {
		return nil, err
	}

	files, err := driver.GetFiles(ctx, taskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return task.CloneFiles(current.Files), nil
		}
		return nil, err
	}

	m.mu.Lock()
	if stored := m.tasks[taskID]; stored != nil {
		stored.Files = task.CloneFiles(files)
		stored.UpdatedAt = time.Now()
	}
	m.mu.Unlock()
	return task.CloneFiles(files), nil
}

// GetPeers 返回任务关联�?peer 列表�?
func (m *Manager) GetPeers(ctx context.Context, gid string) ([]PeerInfo, error) {
	taskID, _, driver, err := m.lookupByGID(gid)
	if err != nil {
		return nil, err
	}

	lister, ok := driver.(PeerLister)
	if !ok {
		return []PeerInfo{}, nil
	}
	peers, err := lister.GetPeers(ctx, taskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return []PeerInfo{}, nil
		}
		return nil, err
	}
	return peers, nil
}

// GetServers 返回任务关联的服务器列表�?
func (m *Manager) GetServers(ctx context.Context, gid string) ([]FileServerInfo, error) {
	taskID, _, driver, err := m.lookupByGID(gid)
	if err != nil {
		return nil, err
	}

	lister, ok := driver.(ServerLister)
	if !ok {
		return []FileServerInfo{}, nil
	}
	servers, err := lister.GetServers(ctx, taskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return []FileServerInfo{}, nil
		}
		return nil, err
	}
	return servers, nil
}

// ChangeOption 动态修改任务选项�?
func (m *Manager) ChangeOption(ctx context.Context, gid string, opts map[string]string) (*task.Task, error) {
	taskID, _, driver, err := m.lookupByGID(gid)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	t := m.tasks[taskID]
	var syncBT bool
	var btEff map[string]string
	var btDriver Driver
	if t != nil {
		if t.LocalOptions != nil {
			for k, v := range opts {
				t.LocalOptions[k] = v
			}
			t.Options = mergeOptions(m.globalOptions, t.LocalOptions)
		} else {
			for k, v := range opts {
				t.Options[k] = v
			}
		}
		if t.Protocol == task.ProtocolBT && optionKeysAffectBT(opts) {
			syncBT = true
			btDriver = m.driverByTaskID[taskID]
			if t.LocalOptions != nil {
				btEff = mergeOptions(m.globalOptions, t.LocalOptions)
			} else {
				btEff = cloneOptions(t.Options)
			}
		}
	}
	m.mu.Unlock()
	if err := driver.ChangeOption(ctx, taskID, cloneOptions(opts)); err != nil {
		return nil, err
	}
	if syncBT {
		if syncer, ok := btDriver.(btTrackerSyncer); ok && btEff != nil {
			_ = syncer.SyncBTTrackerOptions(ctx, taskID, cloneOptions(btEff))
		}
	}
	updated, err := m.tellStatusByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if err := m.SaveSession(ctx); err != nil {
		return nil, err
	}
	m.emit(EventTaskUpdated, updated)
	return updated, nil
}

// GetGlobalOption 返回当前全局选项快照�?
func (m *Manager) GetGlobalOption() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneOptions(m.globalOptions)
}

// ChangeGlobalOption 更新全局选项，并让后续新增任务使用新值�?
func (m *Manager) ChangeGlobalOption(opts map[string]string) map[string]string {
	needBT := false
	for k := range opts {
		if k == "bt-tracker" || k == "bt-exclude-tracker" {
			needBT = true
			break
		}
	}
	type btSyncJob struct {
		taskID string
		eff    map[string]string
		drv    Driver
	}
	var btJobs []btSyncJob

	m.mu.Lock()
	if m.globalOptions == nil {
		m.globalOptions = make(map[string]string)
	}
	for key, value := range opts {
		m.globalOptions[key] = value
		switch key {
		case "dir":
			if strings.TrimSpace(value) != "" {
				m.defaultDir = value
			}
		case "max-concurrent-downloads":
			if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
				m.maxConcurrent = parsed
			}
		case "pause":
			m.startPaused = parsePauseOption(map[string]string{key: value})
		}
	}
	if needBT {
		for id, t := range m.tasks {
			if t == nil || t.Protocol != task.ProtocolBT {
				continue
			}
			var eff map[string]string
			if t.LocalOptions != nil {
				t.Options = mergeOptions(m.globalOptions, t.LocalOptions)
				eff = mergeOptions(m.globalOptions, t.LocalOptions)
			} else {
				for _, k := range []string{"bt-tracker", "bt-exclude-tracker"} {
					if _, ok := opts[k]; ok {
						t.Options[k] = m.globalOptions[k]
					}
				}
				eff = cloneOptions(t.Options)
			}
			btJobs = append(btJobs, btSyncJob{
				taskID: id,
				eff:    cloneOptions(eff),
				drv:    m.driverByTaskID[id],
			})
		}
	}
	out := cloneOptions(m.globalOptions)
	m.mu.Unlock()

	if needBT {
		for _, job := range btJobs {
			if syncer, ok := job.drv.(btTrackerSyncer); ok {
				_ = syncer.SyncBTTrackerOptions(context.Background(), job.taskID, job.eff)
			}
		}
	}
	return out
}

// GetGlobalStat 汇总全局任务统计�?
func (m *Manager) GetGlobalStat() GlobalStat {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var stat GlobalStat
	for _, item := range m.tasks {
		switch item.Status {
		case task.StatusActive:
			stat.NumActive++
		case task.StatusWaiting, task.StatusPaused:
			stat.NumWaiting++
		case task.StatusComplete, task.StatusError, task.StatusRemoved:
			stat.NumStopped++
		}
		stat.DownloadSpeed += item.DownloadSpeed
		stat.UploadSpeed += item.UploadSpeed
	}
	return stat
}

// GetByGID 返回单个任务的副本�?
func (m *Manager) GetByGID(gid string) *task.Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, item := range m.tasks {
		if item.GID == gid {
			return item.Clone()
		}
	}
	return nil
}

// SnapshotTasks 返回当前任务快照，供通知层建立初始状态视图�?
func (m *Manager) SnapshotTasks() []*task.Task {
	return m.snapshotTasks()
}

// LoadSession 从持久化存储恢复任务�?
func (m *Manager) LoadSession(ctx context.Context) error {
	if m.store == nil {
		return nil
	}

	tasks, err := m.store.Load(ctx)
	if err != nil {
		return err
	}

	grouped := make(map[task.Protocol][]*task.Task)
	m.mu.Lock()
	for _, item := range tasks {
		if item.Status == task.StatusRemoved {
			continue
		}
		cloned := item.Clone()
		if cloned.ID == "" {
			cloned.ID = newID()
		}
		if cloned.GID == "" {
			cloned.GID = newGID()
		}
		if cloned.CreatedAt.IsZero() {
			cloned.CreatedAt = time.Now()
		}
		cloned.UpdatedAt = time.Now()
		if cloned.LocalOptions != nil {
			cloned.Options = mergeOptions(m.globalOptions, cloned.LocalOptions)
		}
		m.tasks[cloned.ID] = cloned
		grouped[cloned.Protocol] = append(grouped[cloned.Protocol], cloned.Clone())
	}
	globalSnap := cloneOptions(m.globalOptions)
	m.mu.Unlock()

	for _, driver := range m.snapshotDrivers() {
		sessionDriver, ok := driver.(SessionAwareDriver)
		if !ok {
			continue
		}
		if err := sessionDriver.LoadSessionTasks(ctx, grouped[toProtocol(driver.Name())], globalSnap); err != nil {
			return err
		}
		m.bindDriverToProtocol(driver, toProtocol(driver.Name()))
	}
	for _, item := range m.snapshotTasks() {
		m.emit(EventTaskRestored, item)
	}
	// 恢复后按 max-concurrent 启动仍处于 waiting 的任务（否则 HTTP/BT 等会一直停在队列里）。
	return m.fillSlots(ctx)
}

// SaveSession 将当前任务快照写回存储�?
func (m *Manager) SaveSession(ctx context.Context) error {
	if m.store == nil {
		return nil
	}
	return m.store.Save(ctx, m.snapshotTasks())
}

// Close 在退出前持久化一�?session�?
func (m *Manager) Close(ctx context.Context) error {
	return m.SaveSession(ctx)
}

// Subscribe 允许上层订阅管理器事件�?
func (m *Manager) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = 16
	}

	ch := make(chan Event, buffer)

	m.subMu.Lock()
	id := m.nextSubID
	m.nextSubID++
	m.subscribers[id] = ch
	m.subMu.Unlock()

	unsubscribe := func() {
		m.subMu.Lock()
		if existing, ok := m.subscribers[id]; ok {
			delete(m.subscribers, id)
			close(existing)
		}
		m.subMu.Unlock()
	}
	return ch, unsubscribe
}

// SyncActive 主动刷新 active 任务状态，并在状态推进时发布更新事件�?
func (m *Manager) SyncActive(ctx context.Context) error {
	taskIDs := m.snapshotTaskIDsByStatus(task.StatusActive)
	for _, taskID := range taskIDs {
		updated, err := m.syncTaskByID(ctx, taskID, true)
		if err != nil {
			return err
		}
		if updated != nil && updated.Status == task.StatusComplete {
			if err := m.fillSlots(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) pickDriver(input task.AddTaskInput) (Driver, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, driver := range m.drivers {
		if driver.CanHandle(input) {
			return driver, nil
		}
	}
	return nil, ErrDriverNotFound
}

func (m *Manager) shouldStartImmediately() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := 0
	for _, item := range m.tasks {
		if item.Status == task.StatusActive {
			active++
		}
	}
	return active < m.maxConcurrent
}

func (m *Manager) startTaskByID(ctx context.Context, taskID string) error {
	m.mu.RLock()
	driver := m.driverByTaskID[taskID]
	current := m.tasks[taskID]
	m.mu.RUnlock()

	if current == nil {
		return ErrTaskNotFound
	}
	if driver == nil {
		return ErrDriverNotFound
	}
	if err := driver.Start(ctx, taskID); err != nil {
		return err
	}

	updated, err := driver.TellStatus(ctx, taskID)
	if err != nil {
		return err
	}
	m.storeTask(updated, driver)
	return nil
}

func (m *Manager) syncTaskByID(ctx context.Context, taskID string, emit bool) (*task.Task, error) {
	m.mu.RLock()
	driver := m.driverByTaskID[taskID]
	current := m.tasks[taskID]
	m.mu.RUnlock()

	if current == nil {
		return nil, ErrTaskNotFound
	}
	if driver == nil {
		return current.Clone(), nil
	}

	updated, err := driver.TellStatus(ctx, taskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return current.Clone(), nil
		}
		return nil, err
	}
	updated = m.storeTask(updated, driver)
	if emit {
		m.emit(EventTaskUpdated, updated)
	}
	return updated.Clone(), nil
}

func (m *Manager) pauseTaskByID(ctx context.Context, taskID string, force bool) (*task.Task, error) {
	m.mu.RLock()
	driver := m.driverByTaskID[taskID]
	current := m.tasks[taskID]
	m.mu.RUnlock()

	if current == nil {
		return nil, ErrTaskNotFound
	}
	if driver == nil {
		return current.Clone(), nil
	}

	if err := driver.Pause(ctx, taskID, force); err != nil {
		return nil, err
	}
	updated, err := driver.TellStatus(ctx, taskID)
	if err != nil {
		return nil, err
	}
	updated = m.storeTask(updated, driver)
	if err := m.SaveSession(ctx); err != nil {
		return nil, err
	}
	return updated.Clone(), nil
}

func (m *Manager) unpauseTaskByID(ctx context.Context, taskID string) (*task.Task, error) {
	m.mu.RLock()
	driver := m.driverByTaskID[taskID]
	current := m.tasks[taskID]
	m.mu.RUnlock()

	if current == nil {
		return nil, ErrTaskNotFound
	}
	if driver == nil {
		return current.Clone(), nil
	}

	if err := driver.Start(ctx, taskID); err != nil {
		return nil, err
	}
	updated, err := driver.TellStatus(ctx, taskID)
	if err != nil {
		return nil, err
	}
	updated = m.storeTask(updated, driver)
	if err := m.SaveSession(ctx); err != nil {
		return nil, err
	}
	return updated.Clone(), nil
}

func (m *Manager) tellStatusByID(ctx context.Context, taskID string) (*task.Task, error) {
	return m.syncTaskByID(ctx, taskID, false)
}

func (m *Manager) listByStatuses(ctx context.Context, statuses ...task.Status) ([]*task.Task, error) {
	items, err := m.refreshAll(ctx)
	if err != nil {
		return nil, err
	}

	allowed := make(map[task.Status]struct{}, len(statuses))
	for _, status := range statuses {
		allowed[status] = struct{}{}
	}

	var filtered []*task.Task
	for _, item := range items {
		if _, ok := allowed[item.Status]; ok {
			filtered = append(filtered, item.Clone())
		}
	}
	return filtered, nil
}

func (m *Manager) paginateStatus(ctx context.Context, offset, limit int, statuses ...task.Status) ([]*task.Task, error) {
	if limit <= 0 {
		return []*task.Task{}, nil
	}

	items, err := m.listByStatuses(ctx, statuses...)
	if err != nil {
		return nil, err
	}

	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []*task.Task{}, nil
	}

	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], nil
}

func (m *Manager) refreshAll(ctx context.Context) ([]*task.Task, error) {
	m.mu.RLock()
	taskIDs := make([]string, 0, len(m.tasks))
	for taskID := range m.tasks {
		taskIDs = append(taskIDs, taskID)
	}
	m.mu.RUnlock()

	sort.Strings(taskIDs)

	items := make([]*task.Task, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		item, err := m.tellStatusByID(ctx, taskID)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (m *Manager) lookupByGID(gid string) (string, *task.Task, Driver, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for taskID, item := range m.tasks {
		if item.GID == gid {
			return taskID, item.Clone(), m.driverByTaskID[taskID], nil
		}
	}
	return "", nil, nil, ErrTaskNotFound
}

func (m *Manager) storeTask(updated *task.Task, driver Driver) *task.Task {
	if updated == nil {
		return nil
	}

	cloned := updated.Clone()
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing := m.tasks[cloned.ID]; existing != nil {
		if cloned.GID == "" {
			cloned.GID = existing.GID
		}
		if cloned.Protocol == "" {
			cloned.Protocol = existing.Protocol
		}
		if cloned.Name == "" {
			cloned.Name = existing.Name
		}
		if cloned.SaveDir == "" {
			cloned.SaveDir = existing.SaveDir
		}
		if cloned.CreatedAt.IsZero() {
			cloned.CreatedAt = existing.CreatedAt
		}
		if len(cloned.Files) == 0 {
			cloned.Files = task.CloneFiles(existing.Files)
		}
		if len(cloned.Options) == 0 {
			cloned.Options = cloneOptions(existing.Options)
		}
		if cloned.LocalOptions == nil && existing.LocalOptions != nil {
			cloned.LocalOptions = cloneOptions(existing.LocalOptions)
		}
		if len(cloned.Meta) == 0 {
			cloned.Meta = cloneOptions(existing.Meta)
		}
	}
	cloned.UpdatedAt = time.Now()
	m.tasks[cloned.ID] = cloned
	if driver != nil {
		m.driverByTaskID[cloned.ID] = driver
	}
	return cloned.Clone()
}

func (m *Manager) snapshotTasks() []*task.Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.tasks))
	for taskID := range m.tasks {
		ids = append(ids, taskID)
	}
	sort.Strings(ids)

	snapshot := make([]*task.Task, 0, len(ids))
	for _, taskID := range ids {
		snapshot = append(snapshot, m.tasks[taskID].Clone())
	}
	return snapshot
}

func (m *Manager) snapshotTaskIDsByStatus(statuses ...task.Status) []string {
	allowed := make(map[task.Status]struct{}, len(statuses))
	for _, status := range statuses {
		allowed[status] = struct{}{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var ids []string
	for taskID, item := range m.tasks {
		if _, ok := allowed[item.Status]; ok {
			ids = append(ids, taskID)
		}
	}
	sort.Strings(ids)
	return ids
}

func (m *Manager) snapshotDrivers() []Driver {
	m.mu.RLock()
	defer m.mu.RUnlock()

	drivers := make([]Driver, len(m.drivers))
	copy(drivers, m.drivers)
	return drivers
}

func (m *Manager) bindDriverToProtocol(driver Driver, protocol task.Protocol) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for taskID, item := range m.tasks {
		if item.Protocol == protocol {
			m.driverByTaskID[taskID] = driver
		}
	}
}

func newGID() string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("15040500")))[:16]
	}
	return hex.EncodeToString(raw)
}

func newID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(raw)
}

func cloneOptions(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}

	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mergeOptions(base, overrides map[string]string) map[string]string {
	merged := cloneOptions(base)
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

func (m *Manager) globalOptionsSnapshot() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneOptions(m.globalOptions)
}

func parsePauseOption(options map[string]string) bool {
	value := strings.TrimSpace(options["pause"])
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(strings.ToLower(value))
	if err == nil {
		return parsed
	}
	switch strings.ToLower(value) {
	case "yes":
		return true
	default:
		return false
	}
}

func (m *Manager) fillSlots(ctx context.Context) error {
	startedAny := false
	for {
		if !m.shouldStartImmediately() {
			break
		}

		nextTaskID := m.nextWaitingTaskID()
		if nextTaskID == "" {
			break
		}

		if err := m.startTaskByID(ctx, nextTaskID); err != nil {
			return err
		}
		startedAny = true
		if taskSnapshot, err := m.tellStatusByID(ctx, nextTaskID); err == nil {
			m.emit(EventTaskUpdated, taskSnapshot)
		}
	}
	if startedAny {
		return m.SaveSession(ctx)
	}
	return nil
}

func (m *Manager) nextWaitingTaskID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type candidate struct {
		id        string
		createdAt time.Time
	}

	var candidates []candidate
	for taskID, item := range m.tasks {
		if item.Status != task.StatusWaiting {
			continue
		}
		candidates = append(candidates, candidate{id: taskID, createdAt: item.CreatedAt})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].createdAt.Before(candidates[j].createdAt)
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].id
}

func (m *Manager) emit(eventType EventType, item *task.Task) {
	event := Event{
		Type:       eventType,
		Task:       item.Clone(),
		GlobalStat: m.GetGlobalStat(),
		Time:       time.Now(),
	}

	m.subMu.RLock()
	defer m.subMu.RUnlock()
	for _, ch := range m.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

func toProtocol(name string) task.Protocol {
	switch name {
	case "bt":
		return task.ProtocolBT
	case "ed2k":
		return task.ProtocolED2K
	case "http":
		return task.ProtocolHTTP
	default:
		return task.Protocol(name)
	}
}
