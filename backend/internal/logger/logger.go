package logger

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

// Level represents log severity.
type Level string

const (
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

var std = log.New(os.Stdout, "", 0)

// Entry is the structured JSON log entry.
type Entry struct {
	Timestamp string `json:"timestamp"`
	Level     Level  `json:"level"`
	Message   string `json:"message"`
	// Optional fields
	ErrorCode   string `json:"error_code,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	RawResponse string `json:"raw_response,omitempty"`
	Extra       any    `json:"extra,omitempty"`
}

func write(e Entry) {
	e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	b, _ := json.Marshal(e)
	std.Println(string(b))
}

// Info logs an informational message.
func Info(msg string, extra ...any) {
	e := Entry{Level: LevelInfo, Message: msg}
	if len(extra) > 0 {
		e.Extra = extra[0]
	}
	write(e)
}

// Warn logs a warning message.
func Warn(msg string, extra ...any) {
	e := Entry{Level: LevelWarn, Message: msg}
	if len(extra) > 0 {
		e.Extra = extra[0]
	}
	write(e)
}

// Error logs an error message.
func Error(msg string, extra ...any) {
	e := Entry{Level: LevelError, Message: msg}
	if len(extra) > 0 {
		e.Extra = extra[0]
	}
	write(e)
}

// KISError logs a KIS API error with mandatory fields per CLAUDE.md spec:
// Error Code, Timestamp, and raw KIS API Response Message MUST be included.
func KISError(endpoint, errorCode, rawResponse string) {
	write(Entry{
		Level:       LevelError,
		Message:     "KIS API error",
		Endpoint:    endpoint,
		ErrorCode:   errorCode,
		RawResponse: rawResponse,
	})
}
