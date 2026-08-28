package mchlogcommon_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcommon"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/unittest"
	"github.com/rs/zerolog"
)

func TestMain(m *testing.M) {
	unittest.RunTests(m)
}

func TestGetJSONLogger(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name           string
		content        any
		expectedFields map[string]any
		expectedErr    string
		expectPanic    bool
	}{
		{
			name: "Aceita map[string]any",
			content: map[string]any{
				"mapa1": "aaa",
				"mapa2": 22,
			},
			expectedFields: map[string]any{
				"mapa1": "aaa",
				"mapa2": float64(22),
			},
		},
		{
			name: "Converte map[string]string via reflect",
			content: map[string]string{
				"chave": "valor",
			},
			expectedFields: map[string]any{
				"chave": "valor",
			},
		},
		{
			name: "Converte map[string]int via reflect",
			content: map[string]int{
				"inteiro": 42,
			},
			expectedFields: map[string]any{
				"inteiro": float64(42),
			},
		},
		{
			name: "Converte map[string]float64 via reflect",
			content: map[string]float64{
				"decimal": 1.5,
			},
			expectedFields: map[string]any{
				"decimal": 1.5,
			},
		},
		{
			name: "Ignora valores de tipo não suportado no map",
			content: map[string]bool{
				"booleano": true,
			},
			expectedFields: map[string]any{},
		},
		{
			name:    "Aceita []byte com json válido",
			content: []byte(`{"somekey":{"id":"123"},"tick":10}`),
			expectedFields: map[string]any{
				"somekey": map[string]any{"id": "123"},
				"tick":    float64(10),
			},
		},
		{
			name:        "Rejeita []byte com json inválido",
			content:     []byte(`{invalido}`),
			expectedErr: "tipo inválido do conteúdo do log: []uint8.\nEsperados map, json em formato de string ou []byte",
		},
		{
			name:    "Aceita string com json válido",
			content: `{"testmsg":"some msg"}`,
			expectedFields: map[string]any{
				"testmsg": "some msg",
			},
		},
		{
			name:        "Rejeita string sem json",
			content:     "não é json",
			expectedErr: "tipo inválido do conteúdo do log: string.\nEsperados map, json em formato de string ou []byte",
		},
		{
			name:    "Aceita []any com pares chave/valor",
			content: []any{"key_array1", 22, "key_array2", "55"},
			expectedFields: map[string]any{
				"key_array1": float64(22),
				"key_array2": "55",
			},
		},
		{
			name:    "Converte []string via reflect",
			content: []string{"key_array1", "valor"},
			expectedFields: map[string]any{
				"key_array1": "valor",
			},
		},
		{
			name:    "Converte array de tamanho fixo via reflect",
			content: [2]string{"key_array1", "valor"},
			expectedFields: map[string]any{
				"key_array1": "valor",
			},
		},
		{
			name:        "Rejeita tipo não suportado",
			content:     42,
			expectedErr: "tipo inválido do conteúdo do log: int.\nEsperados map, json em formato de string ou []byte",
		},
		{
			name:        "Entra em panic com conteúdo nulo",
			content:     nil,
			expectPanic: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			var output bytes.Buffer
			zeroLogger := zerolog.New(&output)

			if testCase.expectPanic {
				assert.Panics(func() {
					_, _ = mchlogcommon.GetJSONLogger(&zeroLogger, testCase.content)
				})
				return
			}

			event, err := mchlogcommon.GetJSONLogger(&zeroLogger, testCase.content)

			if testCase.expectedErr != "" {
				assert.Nil(event)
				assert.EqualError(err, testCase.expectedErr)
				return
			}

			assert.NoError(err)
			assert.NotNil(event)

			event.Send()

			var loggedFields map[string]any
			assert.NoError(json.Unmarshal(output.Bytes(), &loggedFields))
			assert.Equal(testCase.expectedFields, loggedFields)
		})
	}
}
