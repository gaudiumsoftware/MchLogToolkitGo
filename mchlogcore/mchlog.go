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
// LogSubject holds RLock during the entire channel send to prevent the channel
// from being closed mid-send.
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
	// Phase 1: under write lock, close the channel and capture references
	// to the old worker so we can wait on it without holding the lock.
	udpMu.Lock()
	oldDone, oldTransport := detachWorkerLocked()
	udpMu.Unlock()

	// Phase 2: wait for the old worker to drain without holding any lock,
	// so LogSubject calls are not blocked during the drain.
	waitAndClose(oldDone, oldTransport)

	// Phase 3: create new transport (network operation, no lock needed)
	t, err := mchloggelf.NewUDPTransport(address, compress)
	if err != nil {
		return err
	}

	// Phase 4: install new worker under write lock
	udpMu.Lock()
	defer udpMu.Unlock()

	// If another SetUDPTarget ran between phase 1 and 4 and installed a new
	// worker, shut it down first (last caller wins). This wait is bounded:
	// the write lock prevents new sends, so the worker only drains what is
	// already in the buffer.
	innerDone, innerTransport := detachWorkerLocked()
	if innerDone != nil {
		<-innerDone
	}
	if innerTransport != nil {
		_ = innerTransport.Close()
	}

	udpTransport = t
	startWorkerLocked()
	return nil
}

// detachWorkerLocked closes the channel and clears the global references,
// returning the old done channel and transport so the caller can wait and
// clean up outside the lock. Must be called with udpMu write lock held.
// After this call, udpChan is nil so LogSubject will skip UDP sends.
func detachWorkerLocked() (<-chan struct{}, *mchloggelf.UDPTransport) {
	if udpChan == nil {
		return nil, nil
	}
	close(udpChan)
	done := udpDone
	transport := udpTransport
	udpChan = nil
	udpDone = nil
	udpTransport = nil
	return done, transport
}

// waitAndClose waits for the worker to finish draining and closes the transport.
// Safe to call with nil arguments (no-op).
func waitAndClose(done <-chan struct{}, transport *mchloggelf.UDPTransport) {
	if done != nil {
		<-done
	}
	if transport != nil {
		_ = transport.Close()
	}
}

// startWorkerLocked initializes the buffered channel and starts a single worker
// goroutine that reads messages and sends them over UDP sequentially.
// Must be called with udpMu write lock held.
func startWorkerLocked() {
	udpChan = make(chan *mchloggelf.GELFMessage, udpBufferSize)
	udpDone = make(chan struct{})

	// Capture references for the goroutine so it operates independently
	// of the global variables. The worker reads from its own channel ref
	// and does not need the mutex.
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

// SetFileOutput enables or disables file-based log output.
// When disabled, logs are only sent via UDP (if configured).
func SetFileOutput(enabled bool) {
	fileOutputEnabled.Store(enabled)
}

// CloseUDP closes the UDP worker and transport connection if active.
// Waits for the worker to drain remaining buffered messages before returning.
func CloseUDP() error {
	udpMu.Lock()
	done, transport := detachWorkerLocked()
	udpMu.Unlock()

	waitAndClose(done, transport)
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

	// Hold RLock for the entire nil-check + send to prevent detachWorkerLocked
	// from closing the channel between the check and the send. The non-blocking
	// select ensures we never block while holding the lock.
	udpMu.RLock()
	defer udpMu.RUnlock()

	if udpChan == nil {
		return
	}

	msg, err := mchloggelf.NewGELFMessage(subject, content, errLog)
	if err != nil {
		log.Printf("[mchlog] failed to create GELF message for subject %q: %v", subject, err)
		return
	}
	select {
	case udpChan <- msg:
		// Message queued successfully
	default:
		log.Printf("[mchlog] UDP send buffer full, dropping GELF message for subject %q", subject)
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

// InitializeMchLog initializes the selected version's backend with the given path.
// If file output is disabled, the file backend is not initialized and no
// directories or files are created.
func InitializeMchLog(path string) {
	versionName := "V1"
	if fileOutputEnabled.Load() {
		if currentVersion == V1 {
			mchlogcorev1.InitializeMchLog(path)
		} else {
			versionName = "V2"
			mchlogcorev2.InitializeMchLog(path)
		}
	}

	// The first log in info should be the version of the logger (v1 or v2)
	MchLog.LogSubject("info", map[string]string{"message": "MchLogToolkit initialized", "version": versionName}, nil)
}
