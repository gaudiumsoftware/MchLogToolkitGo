package mchlogtoolkitgo

import (
	"encoding/json"
	"testing"

	_assert "github.com/stretchr/testify/assert"
)

// formatLog não é exportada, então este arquivo usa o test package interno.
// Ele não pode importar os helpers de unittest (unittest importa este pacote,
// o que fecharia um ciclo); o contrato observável do logger é testado em
// logger_test.go, que usa os helpers a partir do test package externo.
func TestFormatLog(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name      string
		message   string
		level     string
		expectNil bool
	}{
		{
			name:    "Formata uma mensagem de debug",
			message: "debug message",
			level:   DebugLevel,
		},
		{
			name:    "Formata uma mensagem de error",
			message: "error message",
			level:   ErrorLevel,
		},
		{
			name:      "Devolve nil com a mensagem vazia",
			message:   "",
			level:     InfoLevel,
			expectNil: true,
		},
		{
			name:      "Devolve nil com o nível vazio",
			message:   "info message",
			level:     "",
			expectNil: true,
		},
		{
			name:      "Devolve nil com mensagem e nível vazios",
			message:   "",
			level:     "",
			expectNil: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := formatLog(testCase.message, testCase.level)

			if testCase.expectNil {
				assert.Nil(result)
				return
			}

			var formatted map[string]string
			assert.NoError(json.Unmarshal(result, &formatted))

			assert.Equal(testCase.message, formatted["message"])
			assert.Equal(testCase.level, formatted["level"])
			assert.Equal("", formatted["trace"])
			assert.NotEmpty(formatted["source"])
			assert.NotEmpty(formatted["line"])
		})
	}
}
