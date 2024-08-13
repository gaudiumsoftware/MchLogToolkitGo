package mchlogtoolkitgo

import (
	"encoding/json"
	"os"
	"testing"
)

func TestValidLogLevel(t *testing.T) {
	levels := []string{DebugLevel, InfoLevel, WarnLevel, ErrorLevel}
	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			logger, err := NewLogger("test-service", level)
			if err != nil {
				t.Errorf("error creating logger: %v", err)
			}

			if logger.level != level {
				t.Errorf("level is invalid")
			}
		})
	}
}

func TestInvalidLogLevel(t *testing.T) {
	_, err := NewLogger("test-service", "INVALID")
	if err == nil {
		t.Errorf("error was expected")
	}
}

func TestInvalidLoggerFields(t *testing.T) {
	_, err := NewLogger("", "")
	if err == nil {
		t.Errorf("error was expected")
	}
}

func TestSetPath(t *testing.T) {
	logger, err := NewLogger("test-service", DebugLevel)
	if err != nil {
		t.Errorf("error creating logger: %v", err)
	}

	logger.SetPath("test-path")
	if logger.path != "test-path" {
		t.Errorf("path is invalid")
	}

	assertPanic(t, func() { logger.SetPath("") })
}

func TestLogMethods(t *testing.T) {
	defer removeLogFiles(DebugPath)
	serviceName := "test-service"
	logger, err := NewLogger(serviceName, DebugLevel)
	if err != nil {
		t.Errorf("error creating logger: %v", err)
	}
	logger.SetPath(DebugPath)
	logger.Initialize()

	levelFunctions := map[string]func(string){
		DebugLevel: func(m string) { logger.Debug(m) },
		InfoLevel:  func(m string) { logger.Info(m) },
		WarnLevel:  func(m string) { logger.Warn(m) },
		ErrorLevel: func(m string) { logger.Error(m) },
	}

	levels := []string{DebugLevel, InfoLevel, WarnLevel, ErrorLevel}
	messageLevel := map[string]string{
		DebugLevel: "debug message",
		InfoLevel:  "info message",
		WarnLevel:  "warn message",
		ErrorLevel: "error message",
	}
	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			if err = logger.SetLevel(level); err != nil {
				t.Errorf("error setting level: %v", err)
			}

			levelFunctions[level](messageLevel[level])
			logPath := logger.log.GetFileNameFromStreamName(level)
			if _, err = os.Stat(logPath); err != nil {
				t.Errorf("error getting file info: %v", err)
			}

			if err := os.Remove(logPath); err != nil {
				t.Errorf("error removing file: %v", err)
			}

			differentLevel := InfoLevel
			if level == InfoLevel || level == ErrorLevel {
				differentLevel = DebugLevel
			}

			if err = logger.SetLevel(differentLevel); err != nil {
				t.Errorf("error setting level: %v", err)
			}

			levelFunctions[level](messageLevel[level])
			if _, err := os.Stat(logPath); err == nil {
				t.Errorf("file should not exist")
			}
		})
	}
}

func TestInvalidMessages(t *testing.T) {
	logger, err := NewLogger("test-service", DebugLevel)
	if err != nil {
		t.Errorf("error creating logger: %v", err)
	}

	logger.SetPath(DebugPath)
	logger.Initialize()

	levelFunctions := map[string]func(string){
		DebugLevel: logger.Debug,
		InfoLevel:  logger.Info,
		WarnLevel:  logger.Warn,
		ErrorLevel: logger.Error,
	}
	for level, levelMethod := range levelFunctions {
		t.Run(level, func(t *testing.T) {
			if err = logger.SetLevel(level); err != nil {
				t.Errorf("error setting level: %v", err)
			}

			assertPanic(t, func() { levelMethod("") })
		})
	}
}

func removeLogFiles(path string) {
	err := os.RemoveAll(DebugPath)
	if err != nil {
		panic(err)
	}
}

func assertPanic(t *testing.T, f func()) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("panic was expected")
		}
	}()
	f()
}

func TestFormatLogWithValidInput(t *testing.T) {
	message := "test message"
	level := DebugLevel

	result := formatLog(message, level)

	if result == nil {
		t.Errorf("Expected byte array, got nil")
	}

	var log map[string]string
	err := json.Unmarshal(result, &log)
	if err != nil {
		t.Errorf("Error unmarshalling result: %v", err)
	}

	if log["message"] != message {
		t.Errorf("Expected message %s, got %s", message, log["message"])
	}

	if log["level"] != level {
		t.Errorf("Expected level %s, got %s", level, log["level"])
	}
}

func TestFormatLogWithInvalidInput(t *testing.T) {
	message := ""
	level := ""

	result := formatLog(message, level)

	if result != nil {
		t.Errorf("Expected nil, got byte array")
	}
}
