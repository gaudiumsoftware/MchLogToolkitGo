package unittest_test

import (
	"testing"

	"github.com/gaudiumsoftware/mchlogtoolkitgo/unittest"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/unittest/logger"
)

func TestMain(m *testing.M) {
	unittest.RunTests(m)
}

func TestCompareLogs(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name          string
		writeLogs     func()
		expectedLogs  map[string][]string
		expectFailure bool
	}{
		{
			name: "Matches info, debug and error logs after clearing init logs",
			writeLogs: func() {
				logger.Logger.Info("hello from smoke test")
				logger.Logger.Debug("debug from smoke test")
				logger.Logger.Error("error from smoke test")
			},
			expectedLogs: map[string][]string{
				"info":  {"hello from smoke test"},
				"debug": {"debug from smoke test"},
				"error": {"error from smoke test"},
			},
		},
		{
			name: "Length mismatch fails without panicking",
			writeLogs: func() {
				logger.Logger.Info("only one message")
			},
			expectedLogs: map[string][]string{
				"info": {"first", "second"},
			},
			expectFailure: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teardownCase := unittest.SetupTestCase(t)
			defer teardownCase(t)

			tc.writeLogs()

			if tc.expectFailure {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("CompareLogs panicked on length mismatch: %v", r)
					}
				}()

				// Disposable *testing.T, so CompareLogs can call t.Errorf() on the expected mismatch without failing this test.
				// We then assert probe.Failed().
				probe := &testing.T{}
				unittest.CompareLogs(probe, assert, tc.expectedLogs)
				if !probe.Failed() {
					t.Fatal("expected CompareLogs to report a length mismatch failure")
				}
				return
			}

			unittest.CompareLogs(t, assert, tc.expectedLogs)
		})
	}
}
