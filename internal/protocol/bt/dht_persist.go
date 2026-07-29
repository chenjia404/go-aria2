package bt

import (
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/anacrolix/dht/v2"
	torrentlib "github.com/anacrolix/torrent"
)

func resolveDHTNodePath(configured, fallback string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		return configured
	}
	return fallback
}

func loadDHTNodes(client *torrentlib.Client, ipv4Path, ipv6Path string) {
	if client == nil {
		return
	}
	for _, ds := range client.DhtServers() {
		wrapper, ok := ds.(torrentlib.AnacrolixDhtServerWrapper)
		if !ok {
			continue
		}
		path := ipv4Path
		if isIPv6Addr(wrapper.Server.Addr()) {
			path = ipv6Path
		}
		if strings.TrimSpace(path) == "" {
			continue
		}
		if added, err := wrapper.Server.AddNodesFromFile(path); err != nil {
			if !os.IsNotExist(err) {
				log.Printf("[bt] load dht nodes from %s: %v", path, err)
			}
		} else if added > 0 {
			log.Printf("[bt] loaded %d dht nodes from %s", added, path)
		}
	}
}

func saveDHTNodes(client *torrentlib.Client, ipv4Path, ipv6Path string) {
	if client == nil {
		return
	}
	for _, ds := range client.DhtServers() {
		wrapper, ok := ds.(torrentlib.AnacrolixDhtServerWrapper)
		if !ok {
			continue
		}
		path := ipv4Path
		if isIPv6Addr(wrapper.Server.Addr()) {
			path = ipv6Path
		}
		if strings.TrimSpace(path) == "" {
			continue
		}
		nodes := wrapper.Server.Nodes()
		if len(nodes) == 0 {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			log.Printf("[bt] save dht nodes mkdir %s: %v", path, err)
			continue
		}
		if err := dht.WriteNodesToFile(nodes, path); err != nil {
			log.Printf("[bt] save dht nodes to %s: %v", path, err)
		}
	}
}

func isIPv6Addr(addr net.Addr) bool {
	if addr == nil {
		return false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return strings.Contains(addr.String(), ":")
	}
	return strings.Count(host, ":") > 1
}
