// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: mlapointe
// Unit tests for configuration logic

package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Interface != "" {
		t.Errorf("Expected empty interface, got %s", cfg.Interface)
	}
	if cfg.DedupCacheSize != 64000 {
		t.Errorf("Expected cache size 64000, got %d", cfg.DedupCacheSize)
	}
}

func TestLoadConfig(t *testing.T) {
	content := `{
		"interface": "wlan0",
		"dedup_cache_size": 1000
	}`
	tmpFile, err := os.CreateTemp("", "config*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Interface != "wlan0" {
		t.Errorf("Expected wlan0, got %s", cfg.Interface)
	}
	if cfg.DedupCacheSize != 1000 {
		t.Errorf("Expected 1000, got %d", cfg.DedupCacheSize)
	}
	// Check that defaults still apply for missing fields
	if cfg.DedupCacheTTL != 30 {
		t.Errorf("Expected default TTL 30, got %d", cfg.DedupCacheTTL)
	}
}
