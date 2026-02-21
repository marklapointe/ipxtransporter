// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// Log buffering for UI display

package logger

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type LogMessage struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

var (
	messages []LogMessage
	mu       sync.RWMutex
	maxLogs  = 100
)

func Info(format string, v ...any) {
	addLog("INFO", fmt.Sprintf(format, v...))
}

func Error(format string, v ...any) {
	addLog("ERROR", fmt.Sprintf(format, v...))
}

func Fatal(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	addLog("FATAL", msg)
	log.Fatalf("FATAL: %s", msg)
}

func addLog(level, msg string) {
	mu.Lock()
	defer mu.Unlock()

	entry := LogMessage{
		Timestamp: time.Now(),
		Level:     level,
		Message:   msg,
	}
	messages = append(messages, entry)
	if len(messages) > maxLogs {
		messages = messages[1:]
	}

	// Also print to standard log for daemon mode visibility
	log.Printf("%s: %s", level, msg)
}

func GetLogs() []LogMessage {
	mu.RLock()
	defer mu.RUnlock()
	return append([]LogMessage(nil), messages...)
}
