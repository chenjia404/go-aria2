package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/chenjia404/go-aria2/internal/config"
	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

type startupJob struct {
	URIs    []string
	Options map[string]string
}

// bootstrapStartupJobs 在 daemon 启动后注入 aria2 风格的初始任务。
func bootstrapStartupJobs(ctx context.Context, mgr *manager.Manager, cfg *config.Config, opts daemonCLIOptions, logger *log.Logger) error {
	jobs, err := collectStartupJobs(opts)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return nil
	}
	for _, job := range jobs {
		if logger != nil {
			logger.Printf("[INFO] Bootstrapping startup job: %s", strings.Join(job.URIs, ", "))
		}
		input, err := buildStartupInput(cfg, job)
		if err != nil {
			if logger != nil {
				logger.Printf("[WARN] skip startup job %q: %v", strings.Join(job.URIs, ", "), err)
			}
			continue
		}
		if _, err := mgr.Add(ctx, input); err != nil {
			if logger != nil {
				logger.Printf("[WARN] add startup job failed for %q: %v", strings.Join(job.URIs, ", "), err)
			}
			continue
		}
	}
	return nil
}

func collectStartupJobs(opts daemonCLIOptions) ([]startupJob, error) {
	var jobs []startupJob
	startupDefaults := cloneStringMap(opts.startup)
	if strings.TrimSpace(opts.inputFile) != "" {
		fileJobs, err := parseStartupInputFile(opts.inputFile)
		if err != nil {
			return nil, err
		}
		for idx := range fileJobs {
			fileJobs[idx].Options = mergeStartupOptions(startupDefaults, fileJobs[idx].Options)
		}
		jobs = append(jobs, fileJobs...)
	}
	for _, uri := range opts.uris {
		uri = strings.TrimSpace(uri)
		if uri == "" {
			continue
		}
		jobs = append(jobs, startupJob{
			URIs:    []string{uri},
			Options: cloneStringMap(startupDefaults),
		})
	}
	return jobs, nil
}

func parseStartupInputFile(path string) ([]startupJob, error) {
	var r io.Reader
	if path == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}

	scanner := bufio.NewScanner(r)
	var (
		jobs    []startupJob
		current *startupJob
	)
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if isIndented(raw) {
			if current == nil {
				return nil, fmt.Errorf("option without input URI")
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				return nil, fmt.Errorf("invalid input-file option: %q", line)
			}
			key = strings.ToLower(strings.TrimSpace(key))
			value = strings.TrimSpace(value)
			if current.Options == nil {
				current.Options = map[string]string{}
			}
			current.Options[key] = value
			continue
		}

		if current != nil {
			jobs = append(jobs, *current)
		}

		fields := strings.Split(raw, "\t")
		uris := make([]string, 0, len(fields))
		for _, field := range fields {
			field = strings.TrimSpace(field)
			if field != "" {
				uris = append(uris, field)
			}
		}
		if len(uris) == 0 {
			current = nil
			continue
		}
		current = &startupJob{
			URIs:    uris,
			Options: map[string]string{},
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if current != nil {
		jobs = append(jobs, *current)
	}
	return jobs, nil
}

func buildStartupInput(cfg *config.Config, job startupJob) (task.AddTaskInput, error) {
	if len(job.URIs) == 0 {
		return task.AddTaskInput{}, fmt.Errorf("startup job has no URI")
	}

	opts := cloneStringMap(job.Options)
	input := task.AddTaskInput{
		URIs:    append([]string(nil), job.URIs...),
		SaveDir: firstNonEmpty(opts["dir"], cfg.Dir),
		Name:    opts["out"],
		GID:     opts["gid"],
		Options: opts,
		Meta:    map[string]string{"aria2.bootstrap": "true"},
	}
	if input.SaveDir == "" {
		input.SaveDir = cfg.Dir
	}

	if input.Name == "" && len(job.URIs) == 1 {
		if name := filepath.Base(job.URIs[0]); name != "." && name != string(filepath.Separator) {
			input.Name = name
		}
	}

	if len(job.URIs) == 1 {
		if payload, ok, err := loadLocalTorrent(job.URIs[0]); err != nil {
			return task.AddTaskInput{}, err
		} else if ok {
			input.Torrent = payload
			input.URIs = nil
		}
	}
	return input, nil
}

func loadLocalTorrent(uri string) ([]byte, bool, error) {
	trimmed := strings.TrimSpace(uri)
	if trimmed == "" {
		return nil, false, nil
	}
	if strings.Contains(trimmed, "://") {
		return nil, false, nil
	}
	if !strings.HasSuffix(strings.ToLower(trimmed), ".torrent") {
		return nil, false, nil
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if info.IsDir() {
		return nil, false, nil
	}
	payload, err := os.ReadFile(trimmed)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func isIndented(raw string) bool {
	return strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t")
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mergeStartupOptions(defaults, overrides map[string]string) map[string]string {
	merged := cloneStringMap(defaults)
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
