package manager

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

// URIMutator 允许驱动在运行时增删镜像 URI（aria2.changeUri）。
type URIMutator interface {
	ChangeURI(ctx context.Context, taskID string, fileIndex int, delURIs, addURIs []string, position int) (delCount, addCount int, err error)
}

// ChangePosition 调整等待队列中任务的位置，语义对齐 aria2.changePosition。
func (m *Manager) ChangePosition(ctx context.Context, gid string, pos int, how string) (int, error) {
	taskID, item, _, err := m.lookupByGID(gid)
	if err != nil {
		return 0, err
	}
	if item.Status != task.StatusWaiting && item.Status != task.StatusPaused {
		return 0, fmt.Errorf("download is not in queue")
	}

	queue := m.orderedQueueTaskIDs()
	currentIdx := -1
	for i, id := range queue {
		if id == taskID {
			currentIdx = i
			break
		}
	}
	if currentIdx < 0 {
		return 0, fmt.Errorf("download is not in queue")
	}

	newIdx, err := resolveQueueIndex(currentIdx, len(queue), pos, how)
	if err != nil {
		return 0, err
	}
	if newIdx == currentIdx {
		return newIdx, m.SaveSession(ctx)
	}

	reordered := reorderQueueIDs(queue, currentIdx, newIdx)
	m.applyQueueOrder(reordered)
	return newIdx, m.SaveSession(ctx)
}

// ChangeURI 为 HTTP 等支持 URIMutator 的任务增删镜像 URI。
func (m *Manager) ChangeURI(ctx context.Context, gid string, fileIndex int, delURIs, addURIs []string, position int) (int, int, error) {
	taskID, _, driver, err := m.lookupByGID(gid)
	if err != nil {
		return 0, 0, err
	}
	mutator, ok := driver.(URIMutator)
	if !ok {
		return 0, 0, fmt.Errorf("changeUri is not supported for this download")
	}
	delCount, addCount, err := mutator.ChangeURI(ctx, taskID, fileIndex, delURIs, addURIs, position)
	if err != nil {
		return 0, 0, err
	}
	if _, err := m.syncTaskByID(ctx, taskID, true); err != nil {
		return 0, 0, err
	}
	return delCount, addCount, m.SaveSession(ctx)
}

// ForcePauseAll 强制暂停所有活动与等待中的任务（aria2.forcePauseAll）。
func (m *Manager) ForcePauseAll(ctx context.Context) error {
	return m.pauseAll(ctx, true)
}

// LinkBatchDownloads 为同一批次（如 addMetalink）创建的任务设置 aria2 风格关联 GID。
func (m *Manager) LinkBatchDownloads(gids []string) {
	if len(gids) <= 1 {
		return
	}
	leader := gids[0]
	followers := append([]string(nil), gids[1:]...)

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.tasks {
		if item == nil {
			continue
		}
		switch item.GID {
		case leader:
			item.FollowedByGIDs = append([]string(nil), followers...)
		default:
			for _, follower := range followers {
				if item.GID == follower {
					item.FollowingGID = leader
					item.BelongsToGID = leader
					break
				}
			}
		}
	}
}

// insertTaskAtQueuePosition 将 waiting/paused 任务插入队列指定位置（aria2 add* position）。
func (m *Manager) insertTaskAtQueuePosition(gid string, position int) error {
	taskID, item, _, err := m.lookupByGID(gid)
	if err != nil {
		return err
	}
	if item.Status != task.StatusWaiting && item.Status != task.StatusPaused {
		return nil
	}

	queue := m.orderedQueueTaskIDs()
	filtered := make([]string, 0, len(queue))
	for _, id := range queue {
		if id != taskID {
			filtered = append(filtered, id)
		}
	}
	if position < 0 {
		position = len(filtered)
	}
	if position > len(filtered) {
		position = len(filtered)
	}
	reordered := append(append([]string(nil), filtered[:position]...), append([]string{taskID}, filtered[position:]...)...)
	m.applyQueueOrder(reordered)
	return nil
}

// paginateQueueStatus 按 CreatedAt 队列顺序分页返回 waiting/paused 任务。
func (m *Manager) paginateQueueStatus(ctx context.Context, offset, limit int, statuses ...task.Status) ([]*task.Task, error) {
	if limit <= 0 {
		return []*task.Task{}, nil
	}
	items, err := m.listByStatuses(ctx, statuses...)
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].GID < items[j].GID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	if offset < 0 {
		offset = len(items) + offset
		if offset < 0 {
			offset = 0
		}
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

// paginateStopped 按停止时间（UpdatedAt）倒序分页返回 stopped 任务。
func (m *Manager) paginateStopped(ctx context.Context, offset, limit int, statuses ...task.Status) ([]*task.Task, error) {
	if limit <= 0 {
		return []*task.Task{}, nil
	}
	items, err := m.listByStatuses(ctx, statuses...)
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].GID < items[j].GID
		}
		return items[i].UpdatedAt.Before(items[j].UpdatedAt)
	})
	if offset < 0 {
		offset = len(items) + offset
		if offset < 0 {
			offset = 0
		}
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

func (m *Manager) orderedQueueTaskIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type candidate struct {
		id        string
		createdAt time.Time
	}
	var candidates []candidate
	for taskID, item := range m.tasks {
		if item == nil {
			continue
		}
		switch item.Status {
		case task.StatusWaiting, task.StatusPaused:
			candidates = append(candidates, candidate{id: taskID, createdAt: item.CreatedAt})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].createdAt.Equal(candidates[j].createdAt) {
			return candidates[i].id < candidates[j].id
		}
		return candidates[i].createdAt.Before(candidates[j].createdAt)
	})

	out := make([]string, 0, len(candidates))
	for _, item := range candidates {
		out = append(out, item.id)
	}
	return out
}

func (m *Manager) applyQueueOrder(taskIDs []string) {
	base := time.Now().Add(-time.Duration(len(taskIDs)+1) * time.Second)
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, taskID := range taskIDs {
		if item := m.tasks[taskID]; item != nil {
			item.CreatedAt = base.Add(time.Duration(i) * time.Millisecond)
			item.UpdatedAt = time.Now()
		}
	}
}

func resolveQueueIndex(currentIdx, queueLen, pos int, how string) (int, error) {
	switch strings.ToUpper(strings.TrimSpace(how)) {
	case "", "POS_SET":
		return clampQueueIndex(pos, queueLen), nil
	case "POS_CUR":
		return clampQueueIndex(currentIdx+pos, queueLen), nil
	case "POS_END":
		return clampQueueIndex(queueLen-1+pos, queueLen), nil
	default:
		return 0, fmt.Errorf("unknown position mode: %s", how)
	}
}

func clampQueueIndex(index, queueLen int) int {
	if queueLen <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= queueLen {
		return queueLen - 1
	}
	return index
}

func reorderQueueIDs(queue []string, fromIdx, toIdx int) []string {
	if fromIdx == toIdx || len(queue) == 0 {
		return append([]string(nil), queue...)
	}
	out := append([]string(nil), queue...)
	moved := out[fromIdx]
	out = append(out[:fromIdx], out[fromIdx+1:]...)
	if toIdx > fromIdx {
		toIdx--
	}
	return append(out[:toIdx], append([]string{moved}, out[toIdx:]...)...)
}
