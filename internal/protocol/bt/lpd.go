package bt

import (
	"crypto/rand"
	"encoding/binary"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	torrentlib "github.com/anacrolix/torrent"
)

// BEP-14 Local Peer Discovery：UDP 组播 239.192.152.143:6771。
const (
	lpdMulticastAddr = "239.192.152.143:6771"
	lpdMagic         = "\x00BT-SEARCH\x00"
	lpdAnnounceEvery = 60 * time.Second
)

type lpdAnnouncer struct {
	mu         sync.Mutex
	driver     *Driver
	conn       *net.UDPConn
	stopCh     chan struct{}
	stopOnce   sync.Once
	peerID     [20]byte
	listenPort int
}

func (d *Driver) startLPDIfEnabled() *lpdAnnouncer {
	if d == nil || !d.opts.EnableLPD {
		return nil
	}
	port := effectiveListenPort(d.opts)
	if port <= 0 {
		log.Printf("[bt] bt-enable-lpd: listen-port unknown, LPD disabled")
		return nil
	}
	a := &lpdAnnouncer{
		driver:     d,
		stopCh:     make(chan struct{}),
		listenPort: port,
	}
	if _, err := rand.Read(a.peerID[:]); err != nil {
		log.Printf("[bt] bt-enable-lpd: generate peer id: %v", err)
		return nil
	}
	go a.run()
	return a
}

func (a *lpdAnnouncer) stop() {
	if a == nil {
		return
	}
	a.stopOnce.Do(func() {
		close(a.stopCh)
	})
}

func (a *lpdAnnouncer) run() {
	addr, err := net.ResolveUDPAddr("udp4", lpdMulticastAddr)
	if err != nil {
		log.Printf("[bt] bt-enable-lpd: resolve multicast: %v", err)
		return
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		log.Printf("[bt] bt-enable-lpd: listen multicast: %v", err)
		return
	}
	a.mu.Lock()
	a.conn = conn
	a.mu.Unlock()
	defer conn.Close()

	go a.receiveLoop(conn)
	ticker := time.NewTicker(lpdAnnounceEvery)
	defer ticker.Stop()

	a.announceAll()
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.announceAll()
		}
	}
}

func (a *lpdAnnouncer) receiveLoop(conn *net.UDPConn) {
	buf := make([]byte, 512)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-a.stopCh:
				return
			default:
				continue
			}
		}
		a.handlePacket(buf[:n], remote)
	}
}

func (a *lpdAnnouncer) handlePacket(pkt []byte, remote *net.UDPAddr) {
	if len(pkt) < len(lpdMagic)+20+2 {
		return
	}
	if string(pkt[:len(lpdMagic)]) != lpdMagic {
		return
	}
	infoHash := pkt[len(lpdMagic) : len(lpdMagic)+20]
	port := int(binary.BigEndian.Uint16(pkt[len(lpdMagic)+20:]))

	a.driver.mu.RLock()
	tasks := make([]*torrentlib.Torrent, 0, len(a.driver.tasks))
	for _, st := range a.driver.tasks {
		if st != nil && st.torrent != nil && !st.removed {
			tasks = append(tasks, st.torrent)
		}
	}
	a.driver.mu.RUnlock()

	var hash torrentlib.InfoHash
	copy(hash[:], infoHash)
	for _, tor := range tasks {
		if tor.InfoHash() != hash {
			continue
		}
		ip := remote.IP.To4()
		if ip == nil {
			continue
		}
		tor.AddPeers([]torrentlib.PeerInfo{{
			Addr: &net.TCPAddr{IP: ip, Port: port},
		}})
		return
	}
}

func (a *lpdAnnouncer) announceAll() {
	a.driver.mu.RLock()
	tasks := make([]*state, 0, len(a.driver.tasks))
	for _, st := range a.driver.tasks {
		if st != nil && st.torrent != nil && !st.removed && st.started {
			tasks = append(tasks, st)
		}
	}
	a.driver.mu.RUnlock()

	for _, st := range tasks {
		if st != nil && st.options != nil && strings.EqualFold(st.options["bt-enable-lpd"], "false") {
			continue
		}
		a.announceTorrent(st.torrent)
	}
}

func (a *lpdAnnouncer) announceTorrent(tor *torrentlib.Torrent) {
	if tor == nil {
		return
	}
	pkt := make([]byte, 0, len(lpdMagic)+20+2)
	pkt = append(pkt, lpdMagic...)
	pkt = append(pkt, tor.InfoHash().Bytes()...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(a.listenPort))
	pkt = append(pkt, portBytes...)

	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return
	}
	addr, err := net.ResolveUDPAddr("udp4", lpdMulticastAddr)
	if err != nil {
		return
	}
	_, _ = conn.WriteToUDP(pkt, addr)
}
