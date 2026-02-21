// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// Statistics data models and collection for IPXTransporter

package stats

import (
	"fmt"
	"net"
	"sort"
	"time"
)

// Stats holds all metrics that the web API and TUI expose.
type Stats struct {
	TotalReceived  uint64        `json:"total_received"`
	TotalForwarded uint64        `json:"total_forwarded"`
	TotalDropped   uint64        `json:"total_dropped"`
	TotalErrors    uint64        `json:"total_errors"`
	Uptime         time.Duration `json:"uptime"`
	UptimeStr      string        `json:"uptime_str"`
	Peers          []PeerStat    `json:"peers"`
	CaptureError   string        `json:"capture_error"`
	SortField      string        `json:"sort_field"`
	SortReverse    bool          `json:"sort_reverse"`
	ListenAddr     string        `json:"listen_addr"`
	MaxChildren    int           `json:"max_children"`
	DemoProps      *DemoProps    `json:"demo_props,omitzero"`
}

type DemoProps struct {
	PacketRate int `json:"packet_rate"`
	DropRate   int `json:"drop_rate"`
	ErrorRate  int `json:"error_rate"`
	LatencyMs  int `json:"latency_ms"`
	NumPeers   int `json:"num_peers"`
}

func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h >= 24 {
		days := h / 24
		h %= 24
		return fmt.Sprintf("%dd %dh %dm %ds", days, h, m, s)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func (s *Stats) SortPeers() {
	sort.Slice(s.Peers, func(i, j int) bool {
		p1, p2 := s.Peers[i], s.Peers[j]
		var less bool
		switch s.SortField {
		case "id":
			less = p1.ID < p2.ID
		case "ip":
			less = p1.IP.String() < p2.IP.String()
		case "hostname":
			less = p1.Hostname < p2.Hostname
		case "connected":
			less = p1.ConnectedAt.Before(p2.ConnectedAt)
		case "last_seen":
			less = p1.LastSeen.Before(p2.LastSeen)
		case "children":
			less = p1.NumChildren < p2.NumChildren
		case "sent_bytes":
			less = p1.SentBytes < p2.SentBytes
		case "recv_bytes":
			less = p1.RecvBytes < p2.RecvBytes
		case "sent_pkts":
			less = p1.SentPkts < p2.SentPkts
		case "recv_pkts":
			less = p1.RecvPkts < p2.RecvPkts
		case "errors":
			less = p1.Errors < p2.Errors
		default:
			less = p1.ID < p2.ID
		}
		if s.SortReverse {
			return !less
		}
		return less
	})
}

// PeerStat captures traffic & health for an individual peer.
type PeerStat struct {
	ID          string    `json:"id"`
	IP          net.IP    `json:"ip"`
	ConnectedAt time.Time `json:"connected_at"`
	LastSeen    time.Time `json:"last_seen"`
	SentBytes   uint64    `json:"sent_bytes"`
	RecvBytes   uint64    `json:"recv_bytes"`
	SentPkts    uint64    `json:"sent_pkts"`
	RecvPkts    uint64    `json:"recv_pkts"`
	Errors      uint64    `json:"errors"`
	Hostname    string    `json:"hostname"`
	ParentID    string    `json:"parent_id"` // Hierarchical connectivity
	NumChildren int       `json:"num_children"`
	MaxChildren int       `json:"max_children"`
	Country     string    `json:"country"`
	City        string    `json:"city"`
	Lat         float64   `json:"lat"`
	Lon         float64   `json:"lon"`
	Whois       string    `json:"whois"`
}
