package bt

import (
	"strconv"
	"strings"
	"time"
)

const (
	defaultRequestPeerSpeedLimit = 50 * 1024 // aria2 默认 50K
	peerBoostCheckInterval       = time.Second
)

func resolveBTMaxPeers(opts map[string]string, defaultPeers int) int {
	if opts != nil {
		if raw, ok := opts["bt-max-peers"]; ok {
			if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n >= 0 {
				return n
			}
		}
	}
	if defaultPeers > 0 {
		return defaultPeers
	}
	return 0
}

func parseRequestPeerSpeedLimit(opts map[string]string, driverDefault int64) int64 {
	if opts != nil {
		if raw, ok := opts["bt-request-peer-speed-limit"]; ok {
			if v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && v >= 0 {
				return v
			}
		}
	}
	if driverDefault >= 0 {
		return driverDefault
	}
	return defaultRequestPeerSpeedLimit
}

func effectiveRequestPeerThreshold(opts map[string]string, driverDefault int64) int64 {
	threshold := parseRequestPeerSpeedLimit(opts, driverDefault)
	if threshold == 0 {
		return 0
	}
	if opts == nil {
		return threshold
	}
	if raw, ok := opts["max-download-limit"]; ok {
		if maxDL, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && maxDL > 0 && threshold > maxDL {
			return maxDL
		}
	}
	return threshold
}

func parsePositiveLimitOption(opts map[string]string, key string) int64 {
	if opts == nil {
		return 0
	}
	raw, ok := opts[key]
	if !ok {
		return 0
	}
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

func (d *Driver) enforcePeerBoost(st *state, downloadSpeed, uploadSpeed int64) {
	if d == nil || st == nil || st.torrent == nil || !st.started || st.paused || st.removed {
		return
	}
	if st.torrent.Info() == nil {
		return
	}

	now := time.Now()
	if !st.lastPeerBoostAt.IsZero() && now.Sub(st.lastPeerBoostAt) < peerBoostCheckInterval {
		return
	}

	opts := st.options
	threshold := effectiveRequestPeerThreshold(opts, d.opts.RequestPeerSpeedLimit)
	stats := st.torrent.Stats()
	activePeers := stats.ActivePeers
	maxPeers := st.maxPeers
	if maxPeers <= 0 {
		maxPeers = d.opts.MaxPeers
	}

	shouldBoost := false
	if st.torrent.Seeding() {
		if maxPeers > 0 && activePeers >= maxPeers {
			return
		}
		maxUpload := parsePositiveLimitOption(opts, "max-upload-limit")
		if maxUpload == 0 {
			maxUpload = parsePositiveLimitOption(opts, "max-overall-upload-limit")
		}
		if maxUpload == 0 || uploadSpeed < maxUpload*80/100 {
			shouldBoost = true
		}
	} else {
		if threshold == 0 {
			return
		}
		if downloadSpeed < threshold || activePeers == 0 {
			shouldBoost = true
		}
	}

	if !shouldBoost {
		return
	}
	st.lastPeerBoostAt = now
	announceTorrentToDHT(d.client, st.torrent)
}
