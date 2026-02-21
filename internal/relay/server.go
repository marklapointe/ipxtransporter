// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// Main server loop

package relay

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mlapointe/ipxtransporter/internal/capture"
	"github.com/mlapointe/ipxtransporter/internal/config"
	"github.com/mlapointe/ipxtransporter/internal/logger"
	"github.com/mlapointe/ipxtransporter/internal/peer"
	"github.com/mlapointe/ipxtransporter/internal/stats"
)

type Server struct {
	cfg       *config.Config
	capturer  *capture.Capturer
	dedup     *DedupCache
	peers     map[string]*peer.Peer
	peersMu   sync.RWMutex
	startTime time.Time

	totalReceived  uint64
	totalForwarded uint64
	totalDropped   uint64
	totalErrors    uint64
	captureError   atomic.Value // stores string
	configPath     string
	demoMode       bool
	demoPacketRate int
	demoDropRate   int
	demoErrorRate  int
	demoNumPeers   int
	demoPeersMu    sync.RWMutex
	peerRelayChan  chan []byte
}

func NewServer(cfg *config.Config, configPath string) (*Server, error) {
	dedup, err := NewDedupCache(cfg.DedupCacheSize, cfg.DedupCacheTTL)
	if err != nil {
		return nil, err
	}

	return &Server{
		cfg:            cfg,
		configPath:     configPath,
		capturer:       capture.NewCapturer(cfg.Interface),
		dedup:          dedup,
		peers:          make(map[string]*peer.Peer),
		startTime:      time.Now(),
		demoPacketRate: 15,
		demoDropRate:   3,
		demoErrorRate:  10,
		demoNumPeers:   5,
		peerRelayChan:  make(chan []byte, 1000),
	}, nil
}

func (s *Server) Start(ctx context.Context) error {
	if s.demoMode {
		go s.runDemo(ctx)
		return nil
	}
	packetChan := make(chan []byte, 1000)

	if err := s.capturer.Start(ctx, packetChan); err != nil {
		logger.Error("Capture error: %v", err)
		s.captureError.Store(err.Error())
	} else {
		s.captureError.Store("")
	}

	// Listen for incoming peer connections
	go s.listenPeers(ctx, s.peerRelayChan)

	// Outgoing connections to peers
	for _, peerAddr := range s.cfg.Peers {
		go s.connectToPeer(ctx, peerAddr, s.peerRelayChan)
	}

	// Main relay loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case data := <-packetChan:
				atomic.AddUint64(&s.totalReceived, 1)
				if s.dedup.IsDuplicate(data) {
					atomic.AddUint64(&s.totalDropped, 1)
					continue
				}
				s.broadcastToPeers(data)
				atomic.AddUint64(&s.totalForwarded, 1)

			case data := <-s.peerRelayChan:
				if s.dedup.IsDuplicate(data) {
					continue
				}
				if err := s.capturer.Inject(data); err != nil {
					logger.Error("Failed to inject packet: %v", err)
					atomic.AddUint64(&s.totalErrors, 1)
				}
			}
		}
	}()

	return nil
}

func (s *Server) listenPeers(ctx context.Context, relayChan chan<- []byte) {
	var listener net.Listener
	var err error

	if s.cfg.DisableSSL {
		listener, err = net.Listen("tcp", s.cfg.ListenAddr)
	} else {
		cert, err2 := tls.LoadX509KeyPair(s.cfg.TLSCertPath, s.cfg.TLSKeyPath)
		if err2 != nil {
			logger.Error("Failed to load TLS keys: %v", err2)
			return
		}
		tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}
		listener, err = tls.Listen("tcp", s.cfg.ListenAddr, tlsCfg)
	}

	if err != nil {
		logger.Error("Failed to listen: %v", err)
		return
	}
	defer func() {
		if err := listener.Close(); err != nil && err != net.ErrClosed {
			logger.Error("Error closing listener: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		if err := listener.Close(); err != nil && err != net.ErrClosed {
			logger.Error("Error closing listener on context done: %v", err)
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				logger.Error("Accept error: %v", err)
				continue
			}
		}

		s.handleNewConn(ctx, conn, relayChan)
	}
}

func (s *Server) connectToPeer(ctx context.Context, addr string, relayChan chan<- []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			var conn net.Conn
			var err error
			if s.cfg.DisableSSL {
				conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
			} else {
				tlsCfg := &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13} // Production should verify
				conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, tlsCfg)
			}

			if err != nil {
				logger.Error("Failed to connect to peer %s: %v, retrying...", addr, err)
				time.Sleep(5 * time.Second)
				continue
			}

			s.handleNewConn(ctx, conn, relayChan)
			time.Sleep(5 * time.Second) // Wait before reconnecting if it drops
		}
	}
}

func (s *Server) handleNewConn(ctx context.Context, conn net.Conn, relayChan chan<- []byte) {
	peerID := conn.RemoteAddr().String()
	ip, _, _ := net.SplitHostPort(peerID)

	// Enforce bans
	s.peersMu.RLock()
	for _, b := range s.cfg.BannedIDs {
		if b == peerID {
			s.peersMu.RUnlock()
			logger.Info("Rejecting banned peer ID: %s", peerID)
			if err := conn.Close(); err != nil {
				logger.Error("Error closing banned peer ID connection: %v", err)
			}
			return
		}
	}
	for _, b := range s.cfg.BannedHosts {
		if b == ip {
			s.peersMu.RUnlock()
			logger.Info("Rejecting banned peer Host/IP: %s", ip)
			if err := conn.Close(); err != nil {
				logger.Error("Error closing banned peer Host/IP connection: %v", err)
			}
			return
		}
	}
	s.peersMu.RUnlock()

	// Enforce max children for local node
	s.peersMu.RLock()
	localChildren := 0
	for _, p := range s.peers {
		if p.GetStats().ParentID == "Local" {
			localChildren++
		}
	}
	s.peersMu.RUnlock()

	if localChildren >= s.cfg.MaxChildren {
		logger.Info("Rejecting peer %s: max child connections reached (%d)", peerID, s.cfg.MaxChildren)
		if err := conn.Close(); err != nil {
			logger.Error("Error closing peer %s connection (max children): %v", peerID, err)
		}
		return
	}

	p := peer.NewPeer(peerID, conn, s.cfg.NetworkKey)

	s.peersMu.Lock()
	s.peers[peerID] = p
	s.peersMu.Unlock()

	p.Run(ctx, relayChan, func(id string) {
		s.peersMu.Lock()
		delete(s.peers, id)
		s.peersMu.Unlock()
	})
}

func (s *Server) broadcastToPeers(data []byte) {
	s.peersMu.RLock()
	defer s.peersMu.RUnlock()
	for _, p := range s.peers {
		select {
		case p.SendChan <- data:
		default:
			// Peer buffer full, drop packet for this peer
		}
	}
}

func (s *Server) CollectStats() stats.Stats {
	s.peersMu.RLock()
	defer s.peersMu.RUnlock()

	peerStats := make([]stats.PeerStat, 0, len(s.peers))
	for _, p := range s.peers {
		peerStats = append(peerStats, p.GetStats())
	}

	captureErr, _ := s.captureError.Load().(string)
	if s.demoMode && captureErr == "" {
		captureErr = "[DEMO MODE ACTIVE]"
	}

	st := stats.Stats{
		TotalReceived:  atomic.LoadUint64(&s.totalReceived),
		TotalForwarded: atomic.LoadUint64(&s.totalForwarded),
		TotalDropped:   atomic.LoadUint64(&s.totalDropped),
		TotalErrors:    atomic.LoadUint64(&s.totalErrors),
		Uptime:         time.Since(s.startTime),
		UptimeStr:      stats.FormatDuration(time.Since(s.startTime)),
		Peers:          peerStats,
		Logs:           logger.GetLogs(),
		CaptureError:   captureErr,
		SortField:      s.cfg.SortField,
		SortReverse:    s.cfg.SortReverse,
		ListenAddr:     s.cfg.ListenAddr,
		MaxChildren:    s.cfg.MaxChildren,
		NetworkKey:     s.cfg.NetworkKey,
		DemoProps:      nil,
	}

	if s.demoMode {
		st.DemoProps = &stats.DemoProps{
			PacketRate: s.demoPacketRate,
			DropRate:   s.demoDropRate,
			ErrorRate:  s.demoErrorRate,
			NumPeers:   s.demoNumPeers,
		}
	}

	st.SortPeers()
	return st
}

func (s *Server) SetDemoMode(enabled bool) {
	s.demoMode = enabled
}

func (s *Server) SetSortField(field string) {
	s.cfg.SortField = field
	s.persistConfig()
}

func (s *Server) UpdateConfig(adminPass string, maxChildren int, networkKey string) {
	if adminPass != "" {
		s.cfg.AdminPass = adminPass
	}
	if maxChildren > 0 {
		s.cfg.MaxChildren = maxChildren
	}
	if networkKey != "" {
		s.cfg.NetworkKey = networkKey
	}
	s.persistConfig()
}

func (s *Server) persistConfig() {
	if s.configPath != "" {
		if err := config.SaveConfig(s.configPath, s.cfg); err != nil {
			logger.Error("Failed to save config: %v", err)
		}
	}
}

func (s *Server) UpdateDemoProps(packetRate, dropRate, errorRate, numPeers int) {
	s.demoPacketRate = packetRate
	s.demoDropRate = dropRate
	s.demoErrorRate = errorRate
	s.demoNumPeers = numPeers
}

func (s *Server) BanPeer(id string, ip string) {
	s.peersMu.Lock()
	if p, ok := s.peers[id]; ok {
		if err := p.Conn.Close(); err != nil {
			logger.Error("Error closing peer %s connection on ban: %v", id, err)
		}
	}
	s.peersMu.Unlock()

	if id != "" {
		found := false
		for _, b := range s.cfg.BannedIDs {
			if b == id {
				found = true
				break
			}
		}
		if !found {
			s.cfg.BannedIDs = append(s.cfg.BannedIDs, id)
		}
	}
	if ip != "" {
		found := false
		for _, b := range s.cfg.BannedHosts {
			if b == ip {
				found = true
				break
			}
		}
		if !found {
			s.cfg.BannedHosts = append(s.cfg.BannedHosts, ip)
		}
	}

	// Persist config immediately
	s.persistConfig()
}

func (s *Server) DisconnectPeer(id string) {
	s.peersMu.Lock()
	if p, ok := s.peers[id]; ok {
		if err := p.Conn.Close(); err != nil {
			logger.Error("Error closing peer %s connection on disconnect: %v", id, err)
		}
	}
	s.peersMu.Unlock()
}

func (s *Server) AddPeer(ctx context.Context, addr string) {
	// If port is missing, add default port
	if !strings.Contains(addr, "]") { // Not an IPv6 literal with port or without
		if !strings.Contains(addr, ":") {
			addr = net.JoinHostPort(addr, "8787")
		}
	} else {
		// IPv6 literal like [2001:db8::1]
		if !strings.HasSuffix(addr, ":") && !strings.Contains(addr[strings.LastIndex(addr, "]"):], ":") {
			addr = net.JoinHostPort(addr, "8787")
		}
	}

	// Check if already in peers list
	s.peersMu.RLock()
	for _, p := range s.cfg.Peers {
		if p == addr {
			s.peersMu.RUnlock()
			logger.Info("Peer %s already in configuration", addr)
			return
		}
	}
	s.peersMu.RUnlock()

	s.peersMu.Lock()
	s.cfg.Peers = append(s.cfg.Peers, addr)
	s.peersMu.Unlock()

	s.persistConfig()

	if !s.demoMode {
		go s.connectToPeer(ctx, addr, s.peerRelayChan)
	}
	logger.Info("Manually added peer: %s", addr)
}

func (s *Server) runDemo(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Reconcile mock peers
			s.peersMu.Lock()
			currentCount := 0
			for _, p := range s.peers {
				if strings.HasPrefix(p.ID, "demo-node-") {
					currentCount++
				}
			}

			// Update child counts for all peers
			for _, p := range s.peers {
				numChildren := 0
				for _, other := range s.peers {
					if other.GetStats().ParentID == p.ID {
						numChildren++
					}
				}
				p.UpdateChildCount(numChildren, s.cfg.MaxChildren)
			}

			if currentCount < s.demoNumPeers {
				// Add peers
				for i := currentCount; i < s.demoNumPeers; i++ {
					var ip string
					if i%2 == 0 {
						// Generate random publicly routable IPv4 (avoiding 10/8, 172.16/12, 192.168/16, etc.)
						// We'll just use some known public ranges for simplicity or totally random and hope for the best.
						// A better way is to pick a random first octet that isn't private.
						firstOctet := []int{8, 12, 15, 20, 31, 45, 50, 64, 72, 80, 95, 110, 128, 140, 155, 170, 185, 200, 210}[i%19]
						ip = fmt.Sprintf("%d.%d.%d.%d", firstOctet, (i*7)%256, (i*13)%256, (i*17)%256)
					} else {
						// Generate random publicly routable IPv6 (2000::/3 global unicast)
						ip = fmt.Sprintf("2600:%x:%x:%x::%x", (i*7)%65536, (i*13)%65536, (i*17)%65536, i)
					}
					id := fmt.Sprintf("demo-node-%d", i)

					parentID := "Local"
					// Create a hierarchy for demo
					if i > 0 {
						// Attach to a previous node to form a hierarchy
						parentIdx := i / 3 // Roughly 3 children per node
						if parentIdx < i {
							parentID = fmt.Sprintf("demo-node-%d", parentIdx)
						}
					}

					if _, exists := s.peers[id]; !exists {
						p := peer.NewPeer(id, &fakeConn{remoteAddr: &net.TCPAddr{IP: net.ParseIP(ip), Port: 8787}}, s.cfg.NetworkKey)
						p.UpdateDemoStatsWithParent(int64(i), parentID, 0, s.cfg.MaxChildren)
						s.peers[id] = p
					}
				}
			} else if currentCount > s.demoNumPeers {
				// Remove peers
				removed := 0
				toRemove := currentCount - s.demoNumPeers
				for id := range s.peers {
					if removed >= toRemove {
						break
					}
					if strings.HasPrefix(id, "demo-node-") {
						// Don't remove if it has children (to keep tree valid-ish)
						hasChildren := false
						for _, other := range s.peers {
							if other.GetStats().ParentID == id {
								hasChildren = true
								break
							}
						}
						if !hasChildren {
							delete(s.peers, id)
							removed++
						}
					}
				}
			}
			s.peersMu.Unlock()

			atomic.AddUint64(&s.totalReceived, uint64(s.demoPacketRate+int(time.Now().Unix()%int64(s.demoPacketRate/2+1))))
			atomic.AddUint64(&s.totalForwarded, uint64(s.demoPacketRate-s.demoDropRate+int(time.Now().Unix()%int64(s.demoPacketRate/2+1))))
			atomic.AddUint64(&s.totalDropped, uint64(time.Now().Unix()%int64(s.demoDropRate+1)))
			if s.demoErrorRate > 0 && time.Now().Unix()%int64(s.demoErrorRate) == 0 {
				atomic.AddUint64(&s.totalErrors, 1)
			}

			s.peersMu.RLock()
			for _, p := range s.peers {
				p.UpdateDemoStats()
			}
			s.peersMu.RUnlock()
		}
	}
}

func (s *Server) isBasePeer(id string, basePeers []struct{ id, ip string }) bool {
	for _, bp := range basePeers {
		if bp.id == id {
			return true
		}
	}
	return false
}

type fakeConn struct {
	net.Conn
	remoteAddr net.Addr
}

func (f *fakeConn) RemoteAddr() net.Addr { return f.remoteAddr }
func (f *fakeConn) Close() error         { return nil }
