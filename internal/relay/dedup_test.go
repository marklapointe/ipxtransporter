// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// Unit tests for deduplication logic

package relay

import (
	"testing"
)

func TestDedupCache(t *testing.T) {
	cache, err := NewDedupCache(10, 30)
	if err != nil {
		t.Fatalf("Failed to create dedup cache: %v", err)
	}

	packet1 := []byte("packet content 1")
	packet2 := []byte("packet content 2")

	// First time seeing packet1, should not be duplicate
	if cache.IsDuplicate(packet1) {
		t.Error("Expected packet1 to be NOT a duplicate on first arrival")
	}

	// Second time seeing packet1, should be duplicate
	if !cache.IsDuplicate(packet1) {
		t.Error("Expected packet1 to be a duplicate on second arrival")
	}

	// First time seeing packet2, should not be duplicate
	if cache.IsDuplicate(packet2) {
		t.Error("Expected packet2 to be NOT a duplicate on first arrival")
	}

	// Cache eviction test
	cacheSize := 2
	smallCache, _ := NewDedupCache(cacheSize, 30)

	p1 := []byte("p1")
	p2 := []byte("p2")
	p3 := []byte("p3")

	smallCache.IsDuplicate(p1)
	smallCache.IsDuplicate(p2)

	if !smallCache.IsDuplicate(p1) {
		t.Error("Expected p1 to be in cache")
	}

	// Add p3, p1 should be evicted (LRU) or p2 if we just added p1 again
	// smallCache.IsDuplicate(p1) was called, so p2 is now the oldest.
	smallCache.IsDuplicate(p3)

	if !smallCache.IsDuplicate(p3) {
		t.Error("Expected p3 to be in cache")
	}

	// p2 should be evicted
	if smallCache.IsDuplicate(p2) {
		t.Error("Expected p2 to be evicted from cache")
	}
}
