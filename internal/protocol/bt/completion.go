package bt

import (
	"log"
	"os"
	"strings"

	torrentlib "github.com/anacrolix/torrent"
)

func optionEnabled(opts map[string]string, key string) bool {
	if opts == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(opts[key])) {
	case "true", "yes", "1":
		return true
	default:
		return false
	}
}

func (d *Driver) taskOptionEnabled(st *state, key string) bool {
	if st != nil && optionEnabled(st.options, key) {
		return true
	}
	switch key {
	case "bt-detach-seed-only":
		return d.opts.DetachSeedOnly
	case "bt-remove-unselected-file":
		return d.opts.RemoveUnselectedFile
	case "check-integrity":
		return d.opts.CheckIntegrity
	}
	return false
}

func (d *Driver) handleBTCompletionLocked(st *state, taskID string) {
	if st == nil || st.torrent == nil || st.completionHandled {
		return
	}
	if st.torrent.Info() == nil || !st.torrent.Complete().Bool() {
		return
	}
	st.completionHandled = true

	if d.taskOptionEnabled(st, "bt-remove-unselected-file") {
		selectFile := st.selectFile
		tor := st.torrent
		go removeUnselectedFiles(tor, selectFile)
	}
	if d.taskOptionEnabled(st, "bt-detach-seed-only") {
		st.sessionDetached = true
	}
}

func (d *Driver) runCheckIntegrityIfNeeded(taskID string, st *state) {
	if st == nil || st.torrent == nil {
		return
	}
	if !d.taskOptionEnabled(st, "check-integrity") {
		return
	}
	d.startIntegrityCheck(taskID, st)
}

func removeUnselectedFiles(tor *torrentlib.Torrent, selectFile string) {
	if tor == nil || tor.Info() == nil {
		return
	}
	all, set, err := parseAria2SelectFile(selectFile)
	if err != nil || all {
		return
	}
	for i, f := range tor.Files() {
		if _, ok := set[i+1]; ok {
			continue
		}
		path := f.Path()
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("[bt] bt-remove-unselected-file: remove %q: %v", path, err)
		}
	}
}

// SessionDetached 返回任务是否已从 session 持久化中分离（bt-detach-seed-only）。
func (d *Driver) SessionDetached(taskID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	st := d.tasks[taskID]
	return st != nil && st.sessionDetached
}
