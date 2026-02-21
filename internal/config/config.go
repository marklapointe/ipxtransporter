// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: mlapointe
// Configuration management for IPXTransporter

package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Interface      string   `json:"interface"`
	ListenAddr     string   `json:"listen_addr"`
	Peers          []string `json:"peers"`
	TLSCertPath    string   `json:"tls_cert_path"`
	TLSKeyPath     string   `json:"tls_key_path"`
	DisableSSL     bool     `json:"disable_ssl"`
	HTTPListenAddr string   `json:"http_listen_addr"`
	EnableHTTP     bool     `json:"enable_http"`
	LogLevel       string   `json:"log_level"`
	DedupCacheSize int      `json:"dedup_cache_size"`
	DedupCacheTTL  int      `json:"dedup_cache_ttl"`
	SortField      string   `json:"sort_field"`
	SortReverse    bool     `json:"sort_reverse"`
	BannedHosts    []string `json:"banned_hosts"`
	BannedIDs      []string `json:"banned_ids"`
	AdminUser      string   `json:"admin_user"`
	AdminPass      string   `json:"admin_pass"`
	MaxChildren    int      `json:"max_children"`
}

func DefaultConfig() *Config {
	return &Config{
		Interface:      "",
		ListenAddr:     ":8787",
		Peers:          []string{},
		DisableSSL:     false,
		HTTPListenAddr: ":8080",
		EnableHTTP:     true,
		LogLevel:       "info",
		DedupCacheSize: 64000,
		DedupCacheTTL:  30,
		SortField:      "id",
		SortReverse:    false,
		BannedHosts:    []string{},
		BannedIDs:      []string{},
		AdminUser:      "admin",
		AdminPass:      "admin",
		MaxChildren:    5,
	}
}

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
