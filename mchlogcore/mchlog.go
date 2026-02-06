package mchlogcore

import (
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcoreV1"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcoreV2"
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

// SetVersion chooses which version to use (V1 or V2).
// This should ideally be called before InitializeMchLog.
func SetVersion(v LogVersion) {
	currentVersion = v
}

// LogType is the facade structure that delegates calls to either V1 or V2 implementation
type LogType struct{}

// LogSubject records the content to the log file using the selected version
func (l *LogType) LogSubject(subject string, content any, errLog error, ascendStackFrame ...int) {
	if currentVersion == V1 {
		mchlogcoreV1.MchLog.LogSubject(subject, content, errLog, ascendStackFrame...)
	} else {
		mchlogcoreV2.MchLog.LogSubject(subject, content, errLog, ascendStackFrame...)
	}
}

// GetFileNameFromStreamName returns the log file path for the given subject
func (l *LogType) GetFileNameFromStreamName(subject string) string {
	if currentVersion == V1 {
		return mchlogcoreV1.MchLog.GetFileNameFromStreamName(subject)
	} else {
		return mchlogcoreV2.MchLog.GetFileNameFromStreamName(subject)
	}
}

// GetIP returns the IP where the log is running (only available in V1, returns empty for V2)
func (l *LogType) GetIP() string {
	if currentVersion == V1 {
		return mchlogcoreV1.MchLog.GetIP()
	}
	return ""
}

// MchLog is the global instance of the log facade
var MchLog LogType

// InitializeMchLog initializes the selected version's backend with the given path
func InitializeMchLog(path string) {
	versionName := "V1"
	if currentVersion == V1 {
		mchlogcoreV1.InitializeMchLog(path)
	} else {
		versionName = "V2"
		mchlogcoreV2.InitializeMchLog(path)
	}

	// The first log in info should be the version of the logger (v1 or v2)
	MchLog.LogSubject("info", map[string]string{"message": "MchLogToolkit initialized", "version": versionName}, nil)
}
