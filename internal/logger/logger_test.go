// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// Unit tests for logger

package logger

import (
	"testing"
)

func TestLogger(t *testing.T) {
	// Clear existing messages for test isolation
	mu.Lock()
	messages = nil
	mu.Unlock()

	Info("test info %d", 1)
	Error("test error %s", "msg")

	logs := GetLogs()
	if len(logs) != 2 {
		t.Errorf("Expected 2 logs, got %d", len(logs))
	}

	if logs[0].Level != "INFO" || logs[0].Message != "test info 1" {
		t.Errorf("Unexpected first log: %+v", logs[0])
	}

	if logs[1].Level != "ERROR" || logs[1].Message != "test error msg" {
		t.Errorf("Unexpected second log: %+v", logs[1])
	}
}

func TestLoggerBufferLimit(t *testing.T) {
	mu.Lock()
	messages = nil
	maxLogs = 5
	mu.Unlock()
	defer func() {
		mu.Lock()
		maxLogs = 100
		mu.Unlock()
	}()

	for i := 0; i < 10; i++ {
		Info("msg %d", i)
	}

	logs := GetLogs()
	if len(logs) != 5 {
		t.Errorf("Expected 5 logs (limit), got %d", len(logs))
	}

	if logs[0].Message != "msg 5" {
		t.Errorf("Expected first message in buffer to be 'msg 5', got '%s'", logs[0].Message)
	}

	if logs[4].Message != "msg 9" {
		t.Errorf("Expected last message in buffer to be 'msg 9', got '%s'", logs[4].Message)
	}
}
