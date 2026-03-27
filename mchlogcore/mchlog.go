package mchlogcore

import (
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
)

var currentVersion = V1
var udpTransport *mchloggelf.UDPTransport
var fileOutputEnabled = true

// SetVersion chooses which version to use (V1 or V2).
// This should ideally be called before InitializeMchLog.
func SetVersion(v LogVersion) {
	currentVersion = v
}

// SetUDPTarget configures a UDP transport to send GELF messages to the given address.
// The address should be in "host:port" format (e.g., "graylog.example.com:12201").
// If compress is true, messages will be GZIP compressed.
func SetUDPTarget(address string, compress bool) error {
	t, err := mchloggelf.NewUDPTransport(address, compress)
	if err != nil {
		return err
	}
	udpTransport = t
	return nil
}

// SetFileOutput enables or disables file-based log output.
// When disabled, logs are only sent via UDP (if configured).
func SetFileOutput(enabled bool) {
	fileOutputEnabled = enabled
}

// CloseUDP closes the UDP transport connection if one is active.
func CloseUDP() error {
	if udpTransport != nil {
		err := udpTransport.Close()
		udpTransport = nil
		return err
	}
	return nil
}

// LogType is the facade structure that delegates calls to either V1 or V2 implementation
type LogType struct{}

// LogSubject records the content to the log file and/or sends it via UDP using the selected version
func (l *LogType) LogSubject(subject string, content any, errLog error, ascendStackFrame ...int) {
	if fileOutputEnabled {
		if currentVersion == V1 {
			mchlogcorev1.MchLog.LogSubject(subject, content, errLog, ascendStackFrame...)
		} else {
			mchlogcorev2.MchLog.LogSubject(subject, content, errLog, ascendStackFrame...)
		}
	}

	if udpTransport != nil {
		msg, err := mchloggelf.NewGELFMessage(subject, content, errLog)
		if err == nil {
			go udpTransport.Send(msg)
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
