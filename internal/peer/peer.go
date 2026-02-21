// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// Peer connection handling over TLS

package peer

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mlapointe/ipxtransporter/internal/logger"
	"github.com/mlapointe/ipxtransporter/internal/stats"
)

type Peer struct {
	ID          string
	Conn        net.Conn
	ConnectedAt time.Time
	SendChan    chan []byte

	lastSeen    time.Time
	sentBytes   uint64
	recvBytes   uint64
	sentPkts    uint64
	recvPkts    uint64
	errors      uint64
	country     string
	city        string
	lat         float64
	lon         float64
	hostname    string
	parentID    string
	numChildren int
	maxChildren int
	whois       string
	networkKey  string
	mu          sync.RWMutex
}

func NewPeer(id string, conn net.Conn, networkKey string) *Peer {
	return &Peer{
		ID:          id,
		Conn:        conn,
		ConnectedAt: time.Now(),
		SendChan:    make(chan []byte, 1000),
		lastSeen:    time.Now(),
		networkKey:  networkKey,
	}
}

func (p *Peer) Run(ctx context.Context, relayChan chan<- []byte, onDisconnect func(string)) {
	defer func() {
		if err := p.Conn.Close(); err != nil && err != net.ErrClosed {
			logger.Error("Error closing peer %s connection: %v", p.ID, err)
		}
	}()
	defer onDisconnect(p.ID)

	// Authentication Handshake
	if p.networkKey != "" {
		// Send our network key
		keyLen := uint32(len(p.networkKey))
		if err := binary.Write(p.Conn, binary.BigEndian, keyLen); err != nil {
			logger.Error("Peer %s: failed to send key length: %v", p.ID, err)
			return
		}
		if _, err := p.Conn.Write([]byte(p.networkKey)); err != nil {
			logger.Error("Peer %s: failed to send network key: %v", p.ID, err)
			return
		}

		// Receive their network key
		var remoteKeyLen uint32
		if err := binary.Read(p.Conn, binary.BigEndian, &remoteKeyLen); err != nil {
			logger.Error("Peer %s: failed to read remote key length: %v", p.ID, err)
			return
		}
		if remoteKeyLen > 256 {
			logger.Error("Peer %s: remote network key too long (%d)", p.ID, remoteKeyLen)
			return
		}
		remoteKey := make([]byte, remoteKeyLen)
		if _, err := io.ReadFull(p.Conn, remoteKey); err != nil {
			logger.Error("Peer %s: failed to read remote network key: %v", p.ID, err)
			return
		}

		if string(remoteKey) != p.networkKey {
			logger.Error("Peer %s: network key mismatch!", p.ID)
			return
		}
		logger.Info("Peer %s: authenticated successfully", p.ID)
	} else {
		// Even if no key is required locally, we must check if the remote expects one
		// Wait for a short time to see if they send a key length
		p.Conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		var remoteKeyLen uint32
		err := binary.Read(p.Conn, binary.BigEndian, &remoteKeyLen)
		p.Conn.SetReadDeadline(time.Time{}) // Clear deadline

		if err == nil {
			// They sent a key, but we don't have one.
			// Just read it and proceed if we want to be permissive as requested
			// "If there is no network key present, allow anyone to connect"
			if remoteKeyLen <= 256 {
				remoteKey := make([]byte, remoteKeyLen)
				io.ReadFull(p.Conn, remoteKey)
			}
			// Send empty key back if they are waiting for one?
			// Actually, if we are permissive, we should just continue.
			// But the remote might be expecting a key.
			// Let's send an empty key back to satisfy the handshake if they sent one.
			binary.Write(p.Conn, binary.BigEndian, uint32(0))
		}
	}

	// Fetch GeoIP and Whois in background
	go p.lookupInfo()

	wg := sync.WaitGroup{}
	wg.Add(2)

	// Receiver goroutine
	go func() {
		defer wg.Done()
		for {
			// Length-prefixed framing (4 bytes length)
			var length uint32
			err := binary.Read(p.Conn, binary.BigEndian, &length)
			if err != nil {
				if err != io.EOF {
					logger.Error("Peer %s recv error: %v", p.ID, err)
					atomic.AddUint64(&p.errors, 1)
				}
				return
			}

			if length > 2000 { // Max IPX packet is around 576-1500
				logger.Error("Peer %s sent too large packet: %d", p.ID, length)
				return
			}

			data := make([]byte, length)
			_, err = io.ReadFull(p.Conn, data)
			if err != nil {
				logger.Error("Peer %s recv data error: %v", p.ID, err)
				return
			}

			atomic.AddUint64(&p.recvBytes, uint64(length))
			atomic.AddUint64(&p.recvPkts, 1)
			p.mu.Lock()
			p.lastSeen = time.Now()
			p.mu.Unlock()

			select {
			case <-ctx.Done():
				return
			case relayChan <- data:
			}
		}
	}()

	// Sender goroutine
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-p.SendChan:
				if !ok {
					return
				}

				// Write length header
				err := binary.Write(p.Conn, binary.BigEndian, uint32(len(data)))
				if err != nil {
					logger.Error("Peer %s send error: %v", p.ID, err)
					return
				}

				// Write packet data
				_, err = p.Conn.Write(data)
				if err != nil {
					logger.Error("Peer %s send data error: %v", p.ID, err)
					return
				}

				atomic.AddUint64(&p.sentBytes, uint64(len(data)))
				atomic.AddUint64(&p.sentPkts, 1)
			}
		}
	}()

	wg.Wait()
}

func (p *Peer) GetStats() stats.PeerStat {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ip := net.IP{}
	if addr, ok := p.Conn.RemoteAddr().(*net.TCPAddr); ok {
		ip = addr.IP
	}

	return stats.PeerStat{
		ID:          p.ID,
		IP:          ip,
		ConnectedAt: p.ConnectedAt,
		LastSeen:    p.lastSeen,
		SentBytes:   atomic.LoadUint64(&p.sentBytes),
		RecvBytes:   atomic.LoadUint64(&p.recvBytes),
		SentPkts:    atomic.LoadUint64(&p.sentPkts),
		RecvPkts:    atomic.LoadUint64(&p.recvPkts),
		Errors:      atomic.LoadUint64(&p.errors),
		Hostname:    p.hostname,
		ParentID:    p.parentID,
		NumChildren: p.numChildren,
		MaxChildren: p.maxChildren,
		Country:     p.country,
		City:        p.city,
		Lat:         p.lat,
		Lon:         p.lon,
		Whois:       p.whois,
	}
}

func (p *Peer) UpdateDemoStats() {
	p.UpdateDemoStatsWithSeed(time.Now().Unix())
}

func (p *Peer) UpdateDemoStatsWithParent(seed int64, parentID string, numChildren, maxChildren int) {
	p.UpdateDemoStatsWithSeed(seed)
	p.mu.Lock()
	p.parentID = parentID
	p.numChildren = numChildren
	p.maxChildren = maxChildren
	p.mu.Unlock()
}

func (p *Peer) UpdateChildCount(num, max int) {
	p.mu.Lock()
	p.numChildren = num
	p.maxChildren = max
	p.mu.Unlock()
}

func (p *Peer) UpdateDemoStatsWithSeed(seed int64) {
	atomic.AddUint64(&p.sentBytes, uint64(500+seed%1000))
	atomic.AddUint64(&p.recvBytes, uint64(400+seed%1000))
	atomic.AddUint64(&p.sentPkts, uint64(1+seed%5))
	atomic.AddUint64(&p.recvPkts, uint64(1+seed%5))
	p.mu.Lock()
	p.lastSeen = time.Now()
	if p.country == "" {
		p.country = "" // Will be populated by lookupInfo
		p.city = ""
		p.lat = 0
		p.lon = 0
		p.whois = fmt.Sprintf("Demo Whois for %s", p.ID)
	}
	p.mu.Unlock()
}

func (p *Peer) lookupInfo() {
	ip := ""
	if addr, ok := p.Conn.RemoteAddr().(*net.TCPAddr); ok {
		ip = addr.IP.String()
	} else {
		return
	}

	// Use ip-api.com for GeoIP (free for non-commercial, no API key needed)
	resp, err := http.Get(fmt.Sprintf("http://ip-api.com/json/%s", ip))
	if err != nil {
		logger.Error("GeoIP lookup failed: %v", err)
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error("Error closing GeoIP response body: %v", err)
		}
	}()

	var result struct {
		Status  string  `json:"status"`
		Country string  `json:"country"`
		City    string  `json:"city"`
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
		Org     string  `json:"org"`
		AS      string  `json:"as"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Error("Failed to decode GeoIP response: %v", err)
		return
	}

	if result.Status == "success" {
		p.mu.Lock()
		p.country = result.Country
		p.city = result.City
		p.lat = result.Lat
		p.lon = result.Lon
		p.whois = fmt.Sprintf("Org: %s\nAS: %s", result.Org, result.AS)
		p.mu.Unlock()
	}

	// Reverse DNS lookup
	p.mu.RLock()
	currentHostname := p.hostname
	p.mu.RUnlock()

	if currentHostname == "" {
		names, err := net.LookupAddr(ip)
		if err == nil && len(names) > 0 {
			p.mu.Lock()
			p.hostname = strings.TrimSuffix(names[0], ".")
			p.mu.Unlock()
		} else {
			// Fallback to IP address if no hostname found and no demo hostname set
			p.mu.Lock()
			if p.hostname == "" {
				p.hostname = ip
			}
			p.mu.Unlock()
		}
	}
}
