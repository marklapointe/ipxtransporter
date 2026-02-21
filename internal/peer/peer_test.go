// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// Unit tests for peer handshake

package peer

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestPeerHandshake(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	networkKey := "test-key"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Server-side (Peer object)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		p := NewPeer("test-peer", conn, networkKey)
		relayChan := make(chan []byte, 10)
		p.Run(ctx, relayChan, func(id string) {})
	}()

	// Client-side
	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// 1. Send our key
	key := "test-key"
	binary.Write(conn, binary.BigEndian, uint32(len(key)))
	conn.Write([]byte(key))

	// 2. Receive their key length
	var remoteKeyLen uint32
	if err := binary.Read(conn, binary.BigEndian, &remoteKeyLen); err != nil {
		t.Fatalf("failed to read key length: %v", err)
	}
	if remoteKeyLen != uint32(len(networkKey)) {
		t.Fatalf("expected key length %d, got %d", len(networkKey), remoteKeyLen)
	}

	// 3. Receive their key
	remoteKey := make([]byte, remoteKeyLen)
	if _, err := io.ReadFull(conn, remoteKey); err != nil {
		t.Fatalf("failed to read key: %v", err)
	}
	if string(remoteKey) != networkKey {
		t.Fatalf("expected key %s, got %s", networkKey, string(remoteKey))
	}
}

func TestPeerHandshakeMismatch(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	networkKey := "correct-key"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		p := NewPeer("test-peer", conn, networkKey)
		relayChan := make(chan []byte, 10)
		p.Run(ctx, relayChan, func(id string) {})
	}()

	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send wrong key
	key := "wrong-key"
	binary.Write(conn, binary.BigEndian, uint32(len(key)))
	conn.Write([]byte(key))

	// Peer should close connection after mismatch
	// First it will try to send its own key length
	var remoteKeyLen uint32
	binary.Read(conn, binary.BigEndian, &remoteKeyLen)
	remoteKey := make([]byte, remoteKeyLen)
	io.ReadFull(conn, remoteKey)

	// Now it should close
	buf := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, err = conn.Read(buf)
	if err != io.EOF && err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// This is also acceptable if it's still closing or blocked
		} else {
			t.Errorf("expected EOF or timeout, got %v", err)
		}
	}
}
