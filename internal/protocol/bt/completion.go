package bt

import (
	"context"
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

func (d *Driver) handleBTCompletionLocked(st *state, taskID string) {
	if st == nil || st.torrent == nil || st.completionHandled {
		return
	}
	if st.torrent.Info() == nil || !st.torrent.Complete().Bool() {
		return
	}
	st.completionHandled = true

	if optionEnabled(st.options, "bt-remove-unselected-file") {
		removeUnselectedFiles(st.torrent, st.selectFile)
	}
	if optionEnabled(st.options, "bt-detach-seed-only") {
		st.sessionDetached = true
	}
}

func (d *Driver) runCheckIntegrityIfNeeded(st *state) {
	if st == nil || st.torrent == nil {
		return
	}
	check := d.opts.CheckIntegrity || optionEnabled(st.options, "check-integrity")
	if !check {
		return
	}
	tor := st.torrent
	go func() {
		if err := tor.VerifyDataContext(context.Background()); err != nil {
			log.Printf("[bt] check-integrity failed for %s: %v", tor.Name(), err)
		}
	}()
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
