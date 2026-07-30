package bt

import (
	"path/filepath"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"

	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/protocol/common"
)

func torrentStorageForAdd(saveDir string, opts map[string]string) storage.ClientImplCloser {
	indexOut, err := common.ParseIndexOut(opts["index-out"])
	if err != nil || len(indexOut) == 0 {
		return storage.NewFile(saveDir)
	}
	return storage.NewFileOpts(storage.NewFileClientOpts{
		ClientBaseDir: saveDir,
		FilePathMaker: indexOutPathMaker(indexOut),
	})
}

func indexOutPathMaker(indexOut map[int]string) storage.FilePathMaker {
	return func(opts storage.FilePathMakerOpts) string {
		if idx := fileInfoIndex(opts.Info, opts.File); idx > 0 {
			if custom, ok := indexOut[idx]; ok {
				return filepath.Clean(custom)
			}
		}
		var parts []string
		if opts.Info.BestName() != metainfo.NoName {
			parts = append(parts, opts.Info.BestName())
		}
		return filepath.Join(append(parts, opts.File.BestPath()...)...)
	}
}

func fileInfoIndex(info *metainfo.Info, target *metainfo.FileInfo) int {
	if info == nil || target == nil {
		return 0
	}
	for i, fi := range info.UpvertedFiles() {
		if fi.Length != target.Length {
			continue
		}
		if pathsEqual(fi.BestPath(), target.BestPath()) {
			return i + 1
		}
	}
	return 0
}

func pathsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func applyIndexOutPaths(files []task.File, opts map[string]string, saveDir string) {
	for i := range files {
		custom := common.ResolveIndexOutName(opts, files[i].Index, "")
		if custom == "" {
			continue
		}
		if saveDir != "" {
			files[i].Path = filepath.Join(saveDir, filepath.Clean(custom))
		} else {
			files[i].Path = filepath.Clean(custom)
		}
	}
}
