package jsonrpc

import (
	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

// taskSnap 用于检测状态与做种标记变化。
type taskSnap struct {
	Status task.Status
	Seeder bool
}

// aria2NotificationsForEvent 将内部事件映射为 aria2 WebSocket 通知（无 id），与官方手册一致。
func aria2NotificationsForEvent(ev manager.Event, prev map[string]taskSnap) []map[string]any {
	if ev.Task == nil {
		return nil
	}
	gid := ev.Task.GID
	if gid == "" {
		return nil
	}
	cur := ev.Task.Status
	eventObj := map[string]any{"gid": gid}

	switch ev.Type {
	case manager.EventTaskAdded:
		prev[gid] = taskSnap{Status: cur, Seeder: ev.Task.Seeder}
		// 排队中的任务在真正进入 active 时再发 onDownloadStart，避免与 waiting→active 重复。
		if cur == task.StatusWaiting {
			return nil
		}
		return []map[string]any{aria2Notify("aria2.onDownloadStart", eventObj)}
	case manager.EventTaskRemoved:
		delete(prev, gid)
		return []map[string]any{aria2Notify("aria2.onDownloadStop", eventObj)}
	case manager.EventTaskUpdated:
		old := prev[gid]
		next := taskSnap{Status: cur, Seeder: ev.Task.Seeder}
		prev[gid] = next
		return transitionNotifications(ev.Task, old, next)
	default:
		return nil
	}
}

func transitionNotifications(t *task.Task, old, next taskSnap) []map[string]any {
	gid := t.GID
	eventObj := map[string]any{"gid": gid}

	// BT：先做种完成仍属 complete，随后 seeder=false 时再发 onDownloadComplete。
	if old.Status == task.StatusComplete && next.Status == task.StatusComplete &&
		old.Seeder && !next.Seeder && t.Protocol == task.ProtocolBT {
		return []map[string]any{aria2Notify("aria2.onDownloadComplete", eventObj)}
	}

	if old.Status == next.Status && old.Seeder == next.Seeder {
		return nil
	}

	var out []map[string]any
	switch next.Status {
	case task.StatusPaused:
		out = append(out, aria2Notify("aria2.onDownloadPause", eventObj))
	case task.StatusActive:
		if old.Status == task.StatusPaused || old.Status == task.StatusWaiting {
			out = append(out, aria2Notify("aria2.onDownloadStart", eventObj))
		}
	case task.StatusError:
		out = append(out, aria2Notify("aria2.onDownloadError", eventObj))
	case task.StatusComplete:
		if t.Protocol == task.ProtocolBT && next.Seeder {
			out = append(out, aria2Notify("aria2.onBtDownloadComplete", eventObj))
		} else {
			out = append(out, aria2Notify("aria2.onDownloadComplete", eventObj))
		}
	}
	return out
}

func aria2Notify(method string, event map[string]any) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  []any{event},
	}
}
