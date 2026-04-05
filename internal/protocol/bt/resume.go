package bt

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	torrentlib "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

// FastResume 标记任务进入快速恢复模式�?// 该模式由 driver �?torrent metadata 可用时先乐观恢复，再在后台校验�?
func FastResume(item *task.Task) {
	if item == nil {
		return
	}
	if item.Meta == nil {
		item.Meta = map[string]string{}
	}
	item.Meta["bt.resume.mode"] = "fast"
}

// VerifyInBackground 标记任务需要在后台进行严格校验�?
func VerifyInBackground(item *task.Task) {
	if item == nil {
		return
	}
	if item.Meta == nil {
		item.Meta = map[string]string{}
	}
	item.Meta["bt.verify.mode"] = "background"
}

// RebuildBTProgress 依据磁盘上的文件内容重建 BT 任务�?piece completion�?
func RebuildBTProgress(item *task.Task, tor *torrentlib.Torrent) error {
	if item == nil {
		return fmt.Errorf("task is nil")
	}
	if tor == nil {
		return fmt.Errorf("torrent is nil")
	}

	info := tor.Info()
	if info == nil {
		// magnet 任务�?metadata 未就绪前无法严格校验，交给后台处理�?		return nil
	}
	if item.Meta == nil {
		item.Meta = map[string]string{}
	}

	totalPieces := info.NumPieces()
	if totalPieces <= 0 {
		return nil
	}

	completion, err := openPieceCompletion(taskDir(item))
	if err != nil {
		log.Printf("[WARN] open piece completion store failed: %v", err)
		completion = storage.NewMapPieceCompletion()
	}
	if completion != nil {
		defer func() {
			if err := completion.Close(); err != nil {
				log.Printf("[WARN] close piece completion store failed: %v", err)
			}
		}()
	}

	bitfield := make([]byte, (totalPieces+7)/8)
	var (
		completedPieces int
		completedBytes  int64
		firstErr        error
	)

	files := info.UpvertedFiles()
	for pieceIndex := 0; pieceIndex < totalPieces; pieceIndex++ {
		complete, err := verifyPiece(item, info, taskDir(item), pieceIndex, files)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if complete {
			bitfield[pieceIndex/8] |= 1 << (7 - uint(pieceIndex%8))
			completedPieces++
			completedBytes += info.Piece(pieceIndex).Length()
			if completion != nil {
				_ = completion.Set(metainfo.PieceKey{InfoHash: tor.InfoHash(), Index: pieceIndex}, true)
			}
			continue
		}
		if completion != nil {
			_ = completion.Set(metainfo.PieceKey{InfoHash: tor.InfoHash(), Index: pieceIndex}, false)
		}
	}

	item.Meta["bitfield"] = hex.EncodeToString(bitfield)
	item.Meta["bt.totalPieces"] = fmt.Sprintf("%d", totalPieces)
	item.Meta["bt.completedPieces"] = fmt.Sprintf("%d", completedPieces)
	item.PieceLength = info.PieceLength
	item.TotalLength = tor.Length()
	item.CompletedLength = completedBytes
	item.VerifiedLength = completedBytes

	log.Printf("[INFO] Rebuilding BT progress for %s...", displayTaskName(item))
	log.Printf("[INFO] Completed pieces: %d/%d", completedPieces, totalPieces)
	return firstErr
}

func taskDir(item *task.Task) string {
	if item == nil {
		return ""
	}
	return item.SaveDir
}

func displayTaskName(item *task.Task) string {
	if item == nil {
		return "unknown"
	}
	if strings.TrimSpace(item.Name) != "" {
		return item.Name
	}
	return item.GID
}

func verifyPiece(item *task.Task, info *metainfo.Info, saveDir string, pieceIndex int, files []metainfo.FileInfo) (bool, error) {
	expectedHash := info.Piece(pieceIndex).V1Hash()
	if !expectedHash.Ok {
		return false, nil
	}

	pieceLen := info.Piece(pieceIndex).Length()
	if pieceLen <= 0 {
		return false, nil
	}

	pieceBytes := make([]byte, 0, int(pieceLen))
	pieceStart := int64(pieceIndex) * info.PieceLength
	pieceEnd := pieceStart + pieceLen
	if pieceEnd > info.TotalLength() {
		pieceEnd = info.TotalLength()
	}

	for fileIndex, file := range files {
		fileStart := file.TorrentOffset
		fileEnd := fileStart + file.Length
		if fileEnd <= pieceStart || fileStart >= pieceEnd {
			continue
		}
		overlapStart := max64(fileStart, pieceStart)
		overlapEnd := min64(fileEnd, pieceEnd)
		if overlapEnd <= overlapStart {
			continue
		}
		path := filePathForTask(item, saveDir, info, file, fileIndex)
		data, err := readRange(path, overlapStart-fileStart, overlapEnd-overlapStart)
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			if os.IsPermission(err) {
				log.Printf("[WARN] permission denied reading %s: %v", path, err)
				return false, nil
			}
			return false, err
		}
		pieceBytes = append(pieceBytes, data...)
	}

	if int64(len(pieceBytes)) != pieceLen {
		return false, nil
	}
	got := sha1.Sum(pieceBytes)
	return got == expectedHash.Value, nil
}

func filePathFor(saveDir string, info *metainfo.Info, file metainfo.FileInfo) string {
	parts := []string{saveDir}
	if info != nil && info.IsDir() && info.BestName() != metainfo.NoName {
		parts = append(parts, info.BestName())
	}
	if info != nil && !info.IsDir() {
		if info.BestName() != metainfo.NoName {
			parts = append(parts, info.BestName())
		}
	} else {
		parts = append(parts, file.BestPath()...)
	}
	return filepath.Join(parts...)
}

func filePathForTask(item *task.Task, saveDir string, info *metainfo.Info, file metainfo.FileInfo, fileIndex int) string {
	if item != nil && fileIndex >= 0 && fileIndex < len(item.Files) {
		if candidate := strings.TrimSpace(item.Files[fileIndex].Path); candidate != "" {
			if filepath.IsAbs(candidate) {
				return candidate
			}
			return filepath.Join(saveDir, candidate)
		}
	}
	return filePathFor(saveDir, info, file)
}

func readRange(path string, offset, length int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	buf := make([]byte, int(length))
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func openPieceCompletion(saveDir string) (storage.PieceCompletion, error) {
	if strings.TrimSpace(saveDir) == "" {
		return nil, nil
	}
	return storage.NewDefaultPieceCompletionForDir(saveDir)
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
