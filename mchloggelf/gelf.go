package mchloggelf

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// cachedHostname is resolved once and reused for all GELF messages,
// avoiding a syscall per log message in high-throughput scenarios.
var cachedHostname string
var hostnameOnce sync.Once

func getHostname() string {
	hostnameOnce.Do(func() {
		h, err := os.Hostname()
		if err != nil || h == "" {
			cachedHostname = "unknown"
		} else {
			cachedHostname = h
		}
	})
	return cachedHostname
}

// GELF syslog severity levels
const (
	SyslogEmergency     = 0
	SyslogAlert         = 1
	SyslogCritical      = 2
	SyslogError         = 3
	SyslogWarning       = 4
	SyslogNotice        = 5
	SyslogInformational = 6
	SyslogDebug         = 7
)

// GELFMessage represents a GELF 1.1 formatted log message.
// See: https://go2docs.graylog.org/current/getting_in_log_data/gelf.html
type GELFMessage struct {
	Version      string         `json:"version"`
	Host         string         `json:"host"`
	ShortMessage string         `json:"short_message"`
	FullMessage  string         `json:"full_message,omitempty"`
	Timestamp    float64        `json:"timestamp"`
	Level        int            `json:"level"`
	Extra        map[string]any `json:"-"`
}

// MarshalJSON implements custom JSON marshaling that merges standard GELF fields
// with extra fields prefixed with underscore, as required by the GELF spec.
func (m *GELFMessage) MarshalJSON() ([]byte, error) {
	fields := make(map[string]any)

	// Standard GELF fields
	fields["version"] = m.Version
	fields["host"] = m.Host
	fields["short_message"] = m.ShortMessage
	if m.FullMessage != "" {
		fields["full_message"] = m.FullMessage
	}
	fields["timestamp"] = m.Timestamp
	fields["level"] = m.Level

	// Extra fields with underscore prefix
	for k, v := range m.Extra {
		if k == "id" {
			// GELF spec: _id is not allowed
			continue
		}
		fields["_"+k] = v
	}

	return json.Marshal(fields)
}

// LevelToSyslog maps application log level strings to syslog severity levels.
// The level parameter corresponds to the "subject" used in LogSubject, which in
// the standard Logger API is always one of: "fatal", "error", "warn", "info",
// "debug", or "test". If a non-standard subject is passed, it defaults to
// SyslogInformational (6).
func LevelToSyslog(level string) int {
	switch level {
	case "fatal":
		return SyslogEmergency
	case "error":
		return SyslogError
	case "warn":
		return SyslogWarning
	case "info":
		return SyslogInformational
	case "debug", "test":
		return SyslogDebug
	default:
		return SyslogInformational
	}
}

// NewGELFMessage creates a GELF message from the log subject and content.
// The content is expected to be JSON bytes (as produced by formatLog in logger.go)
// or a map[string]any / map[string]string.
func NewGELFMessage(subject string, content any, errLog error) (*GELFMessage, error) {
	msg := &GELFMessage{
		Version:   "1.1",
		Host:      getHostname(),
		Timestamp: float64(time.Now().UnixNano()) / 1e9,
		Level:     LevelToSyslog(subject),
		Extra:     make(map[string]any),
	}

	// Attach application error before any early return so it is never lost
	if errLog != nil {
		msg.Extra["error"] = errLog.Error()
	}

	// Parse content into a map to extract fields
	contentMap, err := contentToMap(content)
	if err != nil {
		// Fallback: use subject as short_message, errLog is already attached above
		msg.ShortMessage = subject
		return msg, nil
	}

	// Extract short_message from the "message" field
	if message, ok := contentMap["message"]; ok {
		msg.ShortMessage = toString(message)
		delete(contentMap, "message")
	} else {
		msg.ShortMessage = subject
	}

	// Extract level from content to avoid duplication (already mapped to syslog level)
	delete(contentMap, "level")

	// All remaining fields become extra fields (prefixed with _ during marshal)
	for k, v := range contentMap {
		msg.Extra[k] = v
	}

	// GELF requires short_message to be non-empty
	if msg.ShortMessage == "" {
		msg.ShortMessage = "(empty)"
	}

	return msg, nil
}

// contentToMap normalizes the content ([]byte, string, map) into map[string]any.
func contentToMap(content any) (map[string]any, error) {
	switch c := content.(type) {
	case []byte:
		var m map[string]any
		if err := json.Unmarshal(c, &m); err != nil {
			return nil, err
		}
		return m, nil
	case string:
		var m map[string]any
		if err := json.Unmarshal([]byte(c), &m); err != nil {
			return nil, err
		}
		return m, nil
	case map[string]any:
		// Make a copy to avoid modifying the original
		m := make(map[string]any, len(c))
		for k, v := range c {
			m[k] = v
		}
		return m, nil
	case map[string]string:
		m := make(map[string]any, len(c))
		for k, v := range c {
			m[k] = v
		}
		return m, nil
	default:
		return nil, fmt.Errorf("unsupported content type: %T", content)
	}
}

// toString converts an interface value to string.
func toString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
