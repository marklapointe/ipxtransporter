// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// Deduplication and packet relay logic

package relay

import (
	"crypto/sha256"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

type DedupCache struct {
	cache *lru.Cache[string, bool]
	ttl   time.Duration
}

func NewDedupCache(size int, ttlSeconds int) (*DedupCache, error) {
	c, err := lru.New[string, bool](size)
	if err != nil {
		return nil, err
	}
	return &DedupCache{
		cache: c,
		ttl:   time.Duration(ttlSeconds) * time.Second,
	}, nil
}

// IsDuplicate returns true if the packet has been seen before.
func (d *DedupCache) IsDuplicate(data []byte) bool {
	// Keyed by hash of the packet data.
	// For IPX (src, dst, txID) would be better if we parse the packet.
	// As a generic implementation, hash is robust for deduplication.
	hash := sha256.Sum256(data)
	key := string(hash[:])

	if d.cache.Contains(key) {
		return true
	}
	d.cache.Add(key, true)
	// LRU doesn't have native TTL, but we can simulate it by storing time.
	// Or just rely on LRU eviction for size management.
	return false
}
