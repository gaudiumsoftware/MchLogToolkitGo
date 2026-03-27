package mchlogcore

import (
	"log"
	"sync"
	"sync/atomic"

	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev1"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev2"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchloggelf"
)

// LogVersion is a type to define which version of the logger to use
type LogVersion int

const (
	// V1 refers to the first version of the logger, which includes IP in filename and uses a channel-based approach
	V1 LogVersion = iota
	// V2 refers to the second version of the logger, which has a simpler file structure without IP or timestamps in names
	V2

	// udpBufferSize is the size of the buffered channel for async UDP sends.
	// Messages beyond this buffer are dropped to prevent memory exhaustion.
	udpBufferSize = 1000
)

var currentVersion = V1

// udpMu protects udpTransport, udpChan, and udpDone from concurrent access.
var udpMu sync.RWMutex
var udpTransport *mchloggelf.UDPTransport
var udpChan chan *mchloggelf.GELFMessage
var udpDone chan struct{}

// fileOutputEnabled uses atomic.Bool for lock-free concurrent reads in LogSubject.
var fileOutputEnabled atomic.Bool

func init() {
	fileOutputEnabled.Store(true)
}

// SetVersion chooses which version to use (V1 or V2).
// This should ideally be called before InitializeMchLog.
func SetVersion(v LogVersion) {
	currentVersion = v
}

// SetUDPTarget configures a UDP transport to send GELF messages to the given address.
// The address should be in "host:port" format (e.g., "graylog.example.com:12201").
// If compress is true, messages will be GZIP compressed.
func SetUDPTarget(address string, compress bool) error {
	udpMu.Lock()
	defer udpMu.Unlock()

	// Close any existing connection to avoid file descriptor leaks
	closeLocked()

	t, err := mchloggelf.NewUDPTransport(address, compress)
	if err != nil {
		return err
	}
	udpTransport = t
	startWorkerLocked()
	return nil
}

// startWorkerLocked initializes the buffered channel and starts a single worker
// goroutine that reads messages and sends them over UDP sequentially.
// Must be called with udpMu held.
func startWorkerLocked() {
	udpChan = make(chan *mchloggelf.GELFMessage, udpBufferSize)
	udpDone = make(chan struct{})

	// Capture references for the goroutine so it doesn't access the global
	// variables after they've been swapped by a subsequent SetUDPTarget call.
	ch := udpChan
	done := udpDone
	transport := udpTransport

	go func() {
		defer close(done)
		for msg := range ch {
			if err := transport.Send(msg); err != nil {
				log.Printf("[mchlog] failed to send GELF message via UDP: %v", err)
			}
		}
	}()
}

// closeLocked drains the channel, waits for the worker to finish,
// and closes the UDP transport. Must be called with udpMu held.
func closeLocked() {
	if udpChan != nil {
		close(udpChan)
		// Release the lock while waiting for the worker to drain,
		// so LogSubject is not blocked. The worker uses its own captured
		// channel reference, so this is safe.
		udpMu.Unlock()
		<-udpDone
		udpMu.Lock()
		udpChan = nil
		udpDone = nil
	}
	if udpTransport != nil {
		_ = udpTransport.Close()
		udpTransport = nil
	}
}

// SetFileOutput enables or disables file-based log output.
// When disabled, logs are only sent via UDP (if configured).
func SetFileOutput(enabled bool) {
	fileOutputEnabled.Store(enabled)
}

// CloseUDP closes the UDP worker and transport connection if active.
func CloseUDP() error {
	udpMu.Lock()
	defer udpMu.Unlock()
	closeLocked()
	return nil
}

// LogType is the facade structure that delegates calls to either V1 or V2 implementation
type LogType struct{}

// LogSubject records the content to the log file and/or sends it via UDP using the selected version
func (l *LogType) LogSubject(subject string, content any, errLog error, ascendStackFrame ...int) {
	if fileOutputEnabled.Load() {
		if currentVersion == V1 {
			mchlogcorev1.MchLog.LogSubject(subject, content, errLog, ascendStackFrame...)
		} else {
			mchlogcorev2.MchLog.LogSubject(subject, content, errLog, ascendStackFrame...)
		}
	}

	// Hold a read lock for the duration of the channel send to prevent
	// the channel from being closed between the nil check and the send.
	udpMu.RLock()
	ch := udpChan
	udpMu.RUnlock()

	if ch != nil {
		msg, err := mchloggelf.NewGELFMessage(subject, content, errLog)
		if err != nil {
			log.Printf("[mchlog] failed to create GELF message for subject %q: %v", subject, err)
			return
		}
		select {
		case ch <- msg:
			// Message queued successfully
		default:
			log.Printf("[mchlog] UDP send buffer full, dropping GELF message for subject %q", subject)
		}
	}
}

// GetFileNameFromStreamName returns the log file path for the given subject
func (l *LogType) GetFileNameFromStreamName(subject string) string {
	if currentVersion == V1 {
		return mchlogcorev1.MchLog.GetFileNameFromStreamName(subject)
	}

	return mchlogcorev2.MchLog.GetFileNameFromStreamName(subject)
}

// GetIP returns the IP where the log is running (only available in V1, returns empty for V2)
func (l *LogType) GetIP() string {
	if currentVersion == V1 {
		return mchlogcorev1.MchLog.GetIP()
	}
	return ""
}

// MchLog is the global instance of the log facade
var MchLog LogType

// InitializeMchLog initializes the selected version's backend with the given path
func InitializeMchLog(path string) {
	versionName := "V1"
	if currentVersion == V1 {
		mchlogcorev1.InitializeMchLog(path)
	} else {
		versionName = "V2"
		mchlogcorev2.InitializeMchLog(path)
	}

	// The first log in info should be the version of the logger (v1 or v2)
	MchLog.LogSubject("info", map[string]string{"message": "MchLogToolkit initialized", "version": versionName}, nil)
}
