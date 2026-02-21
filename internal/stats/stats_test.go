// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// Unit tests for statistics logic

package stats

import (
	"net"
	"testing"
	"time"
)

func TestStats(t *testing.T) {
	now := time.Now()
	ip := net.ParseIP("192.168.1.1")

	peer := PeerStat{
		ID:          "peer-1",
		IP:          ip,
		ConnectedAt: now,
		LastSeen:    now,
		SentBytes:   100,
		RecvBytes:   200,
		SentPkts:    1,
		RecvPkts:    2,
		Errors:      0,
	}

	stats := Stats{
		TotalReceived:  5,
		TotalForwarded: 3,
		TotalDropped:   1,
		TotalErrors:    1,
		Uptime:         time.Hour,
		Peers:          []PeerStat{peer},
	}

	if stats.TotalReceived != 5 {
		t.Errorf("Expected 5 total received packets, got %d", stats.TotalReceived)
	}

	if len(stats.Peers) != 1 {
		t.Fatalf("Expected 1 peer, got %d", len(stats.Peers))
	}

	if stats.Peers[0].ID != "peer-1" {
		t.Errorf("Expected peer ID 'peer-1', got '%s'", stats.Peers[0].ID)
	}

	if !stats.Peers[0].IP.Equal(ip) {
		t.Errorf("Expected peer IP %s, got %s", ip, stats.Peers[0].IP)
	}
}
