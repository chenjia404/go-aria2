package bt

import (
	"fmt"
	"log"
	"net"

	torrentlib "github.com/anacrolix/torrent"
)

func needsSeparateDHTPort(opts Options) bool {
	return opts.EnableDHT &&
		opts.DHTListenPort > 0 &&
		opts.ListenPort > 0 &&
		opts.DHTListenPort != opts.ListenPort
}

func applySeparateDHTConfig(cfg *torrentlib.ClientConfig, opts Options) {
	if needsSeparateDHTPort(opts) {
		cfg.NoDHT = true
	}
}

func attachSeparateDHTServers(client *torrentlib.Client, opts Options) {
	if client == nil || !needsSeparateDHTPort(opts) {
		return
	}

	networks := []string{"udp4"}
	if opts.EnableDHT6 {
		networks = append(networks, "udp6")
	}
	for _, network := range networks {
		addr := fmt.Sprintf(":%d", opts.DHTListenPort)
		conn, err := net.ListenPacket(network, addr)
		if err != nil {
			log.Printf("[bt] dht listen on %s %s failed: %v", network, addr, err)
			continue
		}
		ds, err := client.NewAnacrolixDhtServer(conn)
		if err != nil {
			_ = conn.Close()
			log.Printf("[bt] create dht server on %s failed: %v", addr, err)
			continue
		}
		client.AddDhtServer(torrentlib.AnacrolixDhtServerWrapper{Server: ds})
		log.Printf("[bt] DHT listening on %s (dht-listen-port=%d)", conn.LocalAddr(), opts.DHTListenPort)
	}
}
