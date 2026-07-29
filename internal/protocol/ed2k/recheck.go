package ed2k

import (
	"context"
	"fmt"
	"io"
	"os"

	goed2k "github.com/monkeyWie/goed2k"
	"github.com/monkeyWie/goed2k/protocol"

	"github.com/chenjia404/go-aria2/internal/core/manager"
)

// RecheckEd2kFile 校验 ED2K 任务已下载数据的完整性。
func (d *Driver) RecheckEd2kFile(ctx context.Context, taskID string) error {
	_ = ctx

	d.mu.Lock()
	state := d.tasks[taskID]
	d.mu.Unlock()
	if state == nil || state.removed {
		return manager.ErrTaskNotFound
	}

	handle := d.client.FindTransfer(state.hash)
	if !handle.IsValid() {
		return fmt.Errorf("ed2k transfer not found")
	}

	wasPaused := handle.IsPaused()
	if !wasPaused {
		if err := d.client.PauseTransfer(state.hash); err != nil {
			return err
		}
	}
	defer func() {
		if !wasPaused {
			_ = d.client.ResumeTransfer(state.hash)
		}
	}()

	if handle.IsFinished() {
		computed, err := hashED2KFile(handle.GetFilePath(), handle.GetSize())
		if err != nil {
			return err
		}
		expected := handle.GetHash()
		if !computed.Equal(expected) {
			return fmt.Errorf("ed2k file hash mismatch: expected %s got %s", expected, computed)
		}
		return nil
	}

	transfer := d.client.Session().LookupTransfer(state.hash)
	if transfer == nil {
		return fmt.Errorf("ed2k transfer not found")
	}
	queued := 0
	for _, snap := range handle.PieceSnapshots() {
		if snap.State != goed2k.PieceSnapshotFinished {
			continue
		}
		if transfer.QueuePieceHash(snap.Index) {
			queued++
		}
	}
	if queued == 0 {
		return fmt.Errorf("no finished pieces available for recheck")
	}
	return nil
}

func hashED2KFile(path string, size int64) (protocol.Hash, error) {
	if size <= 0 {
		return protocol.Invalid, fmt.Errorf("invalid file size")
	}
	if size <= goed2k.PieceSize {
		data, err := os.ReadFile(path)
		if err != nil {
			return protocol.Invalid, err
		}
		if int64(len(data)) != size {
			return protocol.Invalid, fmt.Errorf("file size mismatch: expected %d got %d", size, len(data))
		}
		return protocol.HashFromData(data)
	}

	file, err := os.Open(path)
	if err != nil {
		return protocol.Invalid, err
	}
	defer file.Close()

	pieceCount := int((size + goed2k.PieceSize - 1) / goed2k.PieceSize)
	pieceHashes := make([]protocol.Hash, pieceCount)
	for i := 0; i < pieceCount; i++ {
		pieceLen := goed2k.PieceSize
		if i == pieceCount-1 {
			pieceLen = size - int64(i)*goed2k.PieceSize
		}
		buf := make([]byte, pieceLen)
		if _, err := io.ReadFull(file, buf); err != nil {
			return protocol.Invalid, err
		}
		pieceHashes[i], err = protocol.HashFromData(buf)
		if err != nil {
			return protocol.Invalid, err
		}
	}
	return protocol.HashFromHashSet(pieceHashes), nil
}
