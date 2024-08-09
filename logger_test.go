package MchLogToolkitGo

import "testing"

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
	serviceName := "test-service"
	logger, err := NewLogger(serviceName, DebugLevel)
	if err != nil {
		t.Errorf("error creating logger: %v", err)
	}
	logger.SetPath(DebugPath)
	logger.Initialize()

	t.Run(DebugLevel, func(t *testing.T) {
		logger.Debug("debug message")
		//TODO: checar se o log foi gravado no diretório correto e excluir arquivo após teste
	})

	t.Run(InfoLevel, func(t *testing.T) {
		logger.Info("info message")
	})

	t.Run(WarnLevel, func(t *testing.T) {
		logger.Warn("warn message")
	})

	t.Run(ErrorLevel, func(t *testing.T) {
		logger.Error("error message")
	})

	//TODO: excuir arquivos e pastas após teste
}

func assertPanic(t *testing.T, f func()) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("panic was expected")
		}
	}()
	f()
}
