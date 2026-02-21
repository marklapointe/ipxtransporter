// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// Unit tests for server core logic

package relay

import (
	"context"
	"testing"
	"time"

	"github.com/mlapointe/ipxtransporter/internal/config"
)

func TestServerUpdateConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	srv, err := NewServer(cfg, "")
	if err != nil {
		t.Fatal(err)
	}

	srv.UpdateConfig("new-pass", 10, "new-key", false, 60)

	if cfg.AdminPass != "new-pass" {
		t.Errorf("Expected admin pass 'new-pass', got '%s'", cfg.AdminPass)
	}
	if cfg.MaxChildren != 10 {
		t.Errorf("Expected max children 10, got %d", cfg.MaxChildren)
	}
	if cfg.NetworkKey != "new-key" {
		t.Errorf("Expected network key 'new-key', got '%s'", cfg.NetworkKey)
	}
	if cfg.RebalanceEnabled != false {
		t.Error("Expected rebalance disabled")
	}
	if cfg.RebalanceInterval != 60 {
		t.Errorf("Expected rebalance interval 60, got %d", cfg.RebalanceInterval)
	}
}

func TestServerBanPeer(t *testing.T) {
	cfg := config.DefaultConfig()
	srv, err := NewServer(cfg, "")
	if err != nil {
		t.Fatal(err)
	}

	srv.BanPeer("peer-id", "1.2.3.4")

	foundID := false
	for _, id := range cfg.BannedIDs {
		if id == "peer-id" {
			foundID = true
			break
		}
	}
	if !foundID {
		t.Error("peer-id not found in BannedIDs")
	}

	foundHost := false
	for _, host := range cfg.BannedHosts {
		if host == "1.2.3.4" {
			foundHost = true
			break
		}
	}
	if !foundHost {
		t.Error("1.2.3.4 not found in BannedHosts")
	}
}

func TestServerDemoMode(t *testing.T) {
	cfg := config.DefaultConfig()
	srv, err := NewServer(cfg, "")
	if err != nil {
		t.Fatal(err)
	}

	srv.SetDemoMode(true)
	srv.UpdateDemoProps(100, 5, 2, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = srv.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give it a moment to generate some stats
	time.Sleep(1500 * time.Millisecond)

	st := srv.CollectStats()
	if st.TotalReceived == 0 {
		t.Error("Expected some received packets in demo mode")
	}
	if st.DemoProps == nil {
		t.Fatal("Expected DemoProps to be set")
	}
	if st.DemoProps.PacketRate != 100 {
		t.Errorf("Expected packet rate 100, got %d", st.DemoProps.PacketRate)
	}
}
