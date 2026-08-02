package bt

import (
	"context"
	"log"

	torrentlib "github.com/anacrolix/torrent"
)

// torrentIntegritySnapshot 统计已通过 hash 校验的字节数，以及是否仍有分块在校验队列或进行中。
func torrentIntegritySnapshot(tor *torrentlib.Torrent) (verified int64, pending bool) {
	if tor == nil {
		return 0, false
	}
	info := tor.Info()
	if info == nil {
		return 0, false
	}
	for i := 0; i < tor.NumPieces(); i++ {
		ps := tor.PieceState(i)
		if ps.Hashing || ps.QueuedForHash {
			pending = true
		}
		if ps.Complete && ps.Ok {
			verified += int64(info.Piece(i).Length())
		}
	}
	return verified, pending
}

func (d *Driver) startIntegrityCheck(taskID string, st *state) {
	if d == nil || st == nil || st.torrent == nil {
		return
	}
	tor := st.torrent

	d.mu.Lock()
	if st.integrityRunning {
		d.mu.Unlock()
		return
	}
	st.integrityRunning = true
	d.mu.Unlock()

	go func() {
		defer func() {
			d.mu.Lock()
			st.integrityRunning = false
			if verified, _ := torrentIntegritySnapshot(tor); verified > st.verified {
				st.verified = verified
			}
			d.mu.Unlock()
		}()

		sub := tor.SubscribePieceStateChanges()
		defer sub.Close()

		update := func() {
			verified, _ := torrentIntegritySnapshot(tor)
			d.mu.Lock()
			if verified > st.verified {
				st.verified = verified
			}
			d.mu.Unlock()
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			for range sub.Values {
				update()
			}
		}()

		err := tor.VerifyDataContext(context.Background())
		update()
		sub.Close()
		<-done

		if err != nil {
			log.Printf("[bt] integrity check failed for %s: %v", taskID, err)
		}
	}()
}
