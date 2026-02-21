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

	// 1. Add p1
	if smallCache.IsDuplicate(p1) {
		t.Error("p1 should not be duplicate")
	}
	// 2. Add p2
	if smallCache.IsDuplicate(p2) {
		t.Error("p2 should not be duplicate")
	}
	// 3. Mark p2 as newest
	if !smallCache.IsDuplicate(p2) {
		t.Error("p2 should be duplicate")
	}
	// 4. Add p3 (should evict p1)
	if smallCache.IsDuplicate(p3) {
		t.Error("p3 should not be duplicate")
	}

	// 5. Check if p1 was evicted
	if smallCache.IsDuplicate(p1) {
		t.Error("p1 should have been evicted")
	}
}
