package mchlogtoolkitgo_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gaudiumsoftware/mchlogtoolkitgo"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcore"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/unittest"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/unittest/logger"
)

func TestMain(m *testing.M) {
	unittest.RunTests(m)
}

// restoreSharedLogger devolve o logger compartilhado pelos testes ao estado
// esperado por `unittest`: nível debug e logs em <DebugPath>/<ServiceName>/.
// Necessário nos casos que reconfiguram o path ou o nível globais.
func restoreSharedLogger(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		if err := logger.Logger.SetLevel(mchlogtoolkitgo.DebugLevel); err != nil {
			t.Fatalf("Error restoring the shared logger level: %v", err)
		}

		logger.Logger.SetPath(mchlogtoolkitgo.DebugPath)
		logger.Logger.Initialize()
	})
}

func TestNewLogger(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name         string
		service      string
		level        string
		expectedErr  string
		expectLogger bool
	}{
		{
			name:         "Cria o logger com o nível debug",
			service:      "test-service",
			level:        mchlogtoolkitgo.DebugLevel,
			expectLogger: true,
		},
		{
			name:         "Cria o logger com o nível info",
			service:      "test-service",
			level:        mchlogtoolkitgo.InfoLevel,
			expectLogger: true,
		},
		{
			name:         "Cria o logger com o nível warn",
			service:      "test-service",
			level:        mchlogtoolkitgo.WarnLevel,
			expectLogger: true,
		},
		{
			name:         "Cria o logger com o nível error",
			service:      "test-service",
			level:        mchlogtoolkitgo.ErrorLevel,
			expectLogger: true,
		},
		{
			name:         "Cria o logger com o nível fatal",
			service:      "test-service",
			level:        mchlogtoolkitgo.FatalLevel,
			expectLogger: true,
		},
		{
			name:         "Cria o logger com o nível test",
			service:      "test-service",
			level:        mchlogtoolkitgo.TestLevel,
			expectLogger: true,
		},
		{
			name:         "Normaliza o nível informado em caixa alta",
			service:      "test-service",
			level:        "DEBUG",
			expectLogger: true,
		},
		{
			name:        "Rejeita nome de serviço vazio",
			service:     "",
			level:       mchlogtoolkitgo.DebugLevel,
			expectedErr: "service name is required",
		},
		{
			name:        "Rejeita nível desconhecido",
			service:     "test-service",
			level:       "INVALID",
			expectedErr: "invalid log level",
		},
		{
			name:        "Rejeita nível vazio",
			service:     "test-service",
			level:       "",
			expectedErr: "invalid log level",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			log, err := mchlogtoolkitgo.NewLogger(testCase.service, testCase.level)

			if testCase.expectedErr != "" {
				assert.Nil(log)
				assert.EqualError(err, testCase.expectedErr)
				return
			}

			assert.NoError(err)
			assert.NotNil(log)
		})
	}
}

func TestSetPath(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name        string
		path        string
		expectPanic bool
	}{
		{
			name: "Aceita um caminho relativo",
			path: mchlogtoolkitgo.DebugPath,
		},
		{
			name: "Aceita um caminho absoluto",
			path: t.TempDir() + "/",
		},
		{
			name:        "Entra em panic com caminho vazio",
			path:        "",
			expectPanic: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreSharedLogger(t)

			log, err := mchlogtoolkitgo.NewLogger("set-path-service", mchlogtoolkitgo.DebugLevel)
			assert.NoError(err)

			if testCase.expectPanic {
				assert.Panics(func() { log.SetPath(testCase.path) })
				return
			}

			assert.NotPanics(func() { log.SetPath(testCase.path) })

			log.Initialize()
			assert.True(strings.HasPrefix(
				mchlogcore.MchLog.GetFileNameFromStreamName("info"),
				filepath.Join(testCase.path, "set-path-service")+string(filepath.Separator),
			))
		})
	}
}

func TestInitialize(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name    string
		service string
	}{
		{
			name:    "Direciona os logs para o diretório do serviço",
			service: "initialize-service",
		},
		{
			name:    "Redireciona os logs ao ser chamado novamente com outro serviço",
			service: "initialize-other-service",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreSharedLogger(t)

			basePath := t.TempDir() + "/"

			log, err := mchlogtoolkitgo.NewLogger(testCase.service, mchlogtoolkitgo.DebugLevel)
			assert.NoError(err)

			log.SetPath(basePath)
			log.Initialize()

			log.Info("initialize message")

			assert.FileExists(mchlogcore.MchLog.GetFileNameFromStreamName("info"))
			assert.True(strings.HasPrefix(
				mchlogcore.MchLog.GetFileNameFromStreamName("info"),
				filepath.Join(basePath, testCase.service)+string(filepath.Separator),
			))
		})
	}
}

func TestSetLevel(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name        string
		level       string
		expectedErr string
	}{
		{
			name:  "Aceita o nível test",
			level: mchlogtoolkitgo.TestLevel,
		},
		{
			name:  "Aceita o nível debug",
			level: mchlogtoolkitgo.DebugLevel,
		},
		{
			name:  "Aceita o nível info",
			level: mchlogtoolkitgo.InfoLevel,
		},
		{
			name:  "Aceita o nível warn",
			level: mchlogtoolkitgo.WarnLevel,
		},
		{
			name:  "Aceita o nível error",
			level: mchlogtoolkitgo.ErrorLevel,
		},
		{
			name:  "Aceita o nível fatal",
			level: mchlogtoolkitgo.FatalLevel,
		},
		{
			name:  "Aceita o nível em caixa alta",
			level: "WARN",
		},
		{
			name:        "Rejeita o nível vazio",
			level:       "",
			expectedErr: "invalid log level",
		},
		{
			name:        "Rejeita um nível desconhecido",
			level:       "verbose",
			expectedErr: "invalid log level",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			log, err := mchlogtoolkitgo.NewLogger("set-level-service", mchlogtoolkitgo.DebugLevel)
			assert.NoError(err)

			err = log.SetLevel(testCase.level)

			if testCase.expectedErr != "" {
				assert.EqualError(err, testCase.expectedErr)
				return
			}

			assert.NoError(err)
		})
	}
}

func TestTest(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name         string
		level        string
		message      string
		expectedLogs map[string][]string
		expectPanic  bool
	}{
		{
			name:    "Grava quando o nível é test",
			level:   mchlogtoolkitgo.TestLevel,
			message: "test message",
			expectedLogs: map[string][]string{
				"test": {"test message"},
			},
		},
		{
			name:         "Não grava quando o nível é debug",
			level:        mchlogtoolkitgo.DebugLevel,
			message:      "test message",
			expectedLogs: map[string][]string{},
		},
		{
			name:         "Não grava quando o nível é info",
			level:        mchlogtoolkitgo.InfoLevel,
			message:      "test message",
			expectedLogs: map[string][]string{},
		},
		{
			name:        "Entra em panic com mensagem vazia",
			level:       mchlogtoolkitgo.TestLevel,
			message:     "",
			expectPanic: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreSharedLogger(t)
			assert.NoError(logger.Logger.SetLevel(testCase.level))

			if testCase.expectPanic {
				assert.Panics(func() { logger.Logger.Test(testCase.message) })
				return
			}

			logger.Logger.Test(testCase.message)
			unittest.CompareLogs(t, assert, testCase.expectedLogs)
		})
	}
}

func TestDebug(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name         string
		level        string
		message      string
		expectedLogs map[string][]string
		expectPanic  bool
	}{
		{
			name:    "Grava quando o nível é debug",
			level:   mchlogtoolkitgo.DebugLevel,
			message: "debug message",
			expectedLogs: map[string][]string{
				"debug": {"debug message"},
			},
		},
		{
			name:         "Não grava quando o nível é info",
			level:        mchlogtoolkitgo.InfoLevel,
			message:      "debug message",
			expectedLogs: map[string][]string{},
		},
		{
			name:         "Não grava quando o nível é error",
			level:        mchlogtoolkitgo.ErrorLevel,
			message:      "debug message",
			expectedLogs: map[string][]string{},
		},
		{
			name:        "Entra em panic com mensagem vazia",
			level:       mchlogtoolkitgo.DebugLevel,
			message:     "",
			expectPanic: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreSharedLogger(t)
			assert.NoError(logger.Logger.SetLevel(testCase.level))

			if testCase.expectPanic {
				assert.Panics(func() { logger.Logger.Debug(testCase.message) })
				return
			}

			logger.Logger.Debug(testCase.message)
			unittest.CompareLogs(t, assert, testCase.expectedLogs)
		})
	}
}

func TestWarn(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name         string
		level        string
		message      string
		expectedLogs map[string][]string
		expectPanic  bool
	}{
		{
			name:    "Grava quando o nível é warn",
			level:   mchlogtoolkitgo.WarnLevel,
			message: "warn message",
			expectedLogs: map[string][]string{
				"warn": {"warn message"},
			},
		},
		{
			name:    "Grava quando o nível é debug",
			level:   mchlogtoolkitgo.DebugLevel,
			message: "warn message",
			expectedLogs: map[string][]string{
				"warn": {"warn message"},
			},
		},
		{
			name:         "Não grava quando o nível é info",
			level:        mchlogtoolkitgo.InfoLevel,
			message:      "warn message",
			expectedLogs: map[string][]string{},
		},
		{
			name:        "Entra em panic com mensagem vazia",
			level:       mchlogtoolkitgo.WarnLevel,
			message:     "",
			expectPanic: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreSharedLogger(t)
			assert.NoError(logger.Logger.SetLevel(testCase.level))

			if testCase.expectPanic {
				assert.Panics(func() { logger.Logger.Warn(testCase.message) })
				return
			}

			logger.Logger.Warn(testCase.message)
			unittest.CompareLogs(t, assert, testCase.expectedLogs)
		})
	}
}

func TestInfo(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name         string
		level        string
		message      string
		expectedLogs map[string][]string
		expectPanic  bool
	}{
		{
			name:    "Grava quando o nível é info",
			level:   mchlogtoolkitgo.InfoLevel,
			message: "info message",
			expectedLogs: map[string][]string{
				"info": {"info message"},
			},
		},
		{
			name:    "Grava mesmo quando o nível é error",
			level:   mchlogtoolkitgo.ErrorLevel,
			message: "info message",
			expectedLogs: map[string][]string{
				"info": {"info message"},
			},
		},
		{
			name:    "Grava várias mensagens em sequência",
			level:   mchlogtoolkitgo.InfoLevel,
			message: "info message",
			expectedLogs: map[string][]string{
				"info": {"info message"},
			},
		},
		{
			name:        "Entra em panic com mensagem vazia",
			level:       mchlogtoolkitgo.InfoLevel,
			message:     "",
			expectPanic: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreSharedLogger(t)
			assert.NoError(logger.Logger.SetLevel(testCase.level))

			if testCase.expectPanic {
				assert.Panics(func() { logger.Logger.Info(testCase.message) })
				return
			}

			logger.Logger.Info(testCase.message)
			unittest.CompareLogs(t, assert, testCase.expectedLogs)
		})
	}
}

func TestError(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name         string
		level        string
		message      string
		expectedLogs map[string][]string
		expectPanic  bool
	}{
		{
			name:    "Grava quando o nível é error",
			level:   mchlogtoolkitgo.ErrorLevel,
			message: "error message",
			expectedLogs: map[string][]string{
				"error": {"error message"},
			},
		},
		{
			name:    "Grava mesmo quando o nível é info",
			level:   mchlogtoolkitgo.InfoLevel,
			message: "error message",
			expectedLogs: map[string][]string{
				"error": {"error message"},
			},
		},
		{
			name:        "Entra em panic com mensagem vazia",
			level:       mchlogtoolkitgo.ErrorLevel,
			message:     "",
			expectPanic: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreSharedLogger(t)
			assert.NoError(logger.Logger.SetLevel(testCase.level))

			if testCase.expectPanic {
				assert.Panics(func() { logger.Logger.Error(testCase.message) })
				return
			}

			logger.Logger.Error(testCase.message)
			unittest.CompareLogs(t, assert, testCase.expectedLogs)
		})
	}
}

func TestFatal(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name         string
		level        string
		message      string
		expectedLogs map[string][]string
		expectPanic  bool
	}{
		{
			name:    "Grava quando o nível é fatal",
			level:   mchlogtoolkitgo.FatalLevel,
			message: "fatal message",
			expectedLogs: map[string][]string{
				"fatal": {"fatal message"},
			},
		},
		{
			name:    "Grava mesmo quando o nível é debug",
			level:   mchlogtoolkitgo.DebugLevel,
			message: "fatal message",
			expectedLogs: map[string][]string{
				"fatal": {"fatal message"},
			},
		},
		{
			name:        "Entra em panic com mensagem vazia",
			level:       mchlogtoolkitgo.FatalLevel,
			message:     "",
			expectPanic: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			restoreSharedLogger(t)
			assert.NoError(logger.Logger.SetLevel(testCase.level))

			if testCase.expectPanic {
				assert.Panics(func() { logger.Logger.Fatal(testCase.message) })
				return
			}

			logger.Logger.Fatal(testCase.message)
			unittest.CompareLogs(t, assert, testCase.expectedLogs)
		})
	}
}
