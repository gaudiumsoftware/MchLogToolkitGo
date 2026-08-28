package mchlogcorev2_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gaudiumsoftware/mchlogtoolkitgo"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcore"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/mchlogcorev2"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/unittest"
	"github.com/gaudiumsoftware/mchlogtoolkitgo/unittest/logger"
)

// basePath é o diretório usado pelo logger compartilhado dos testes.
// Como o mchlogcorev2 mantém um cache de loggers por subject (que segura o
// arquivo aberto durante toda a vida do processo), os testes só reinicializam
// o pacote em TestInitializeMchLog — e lá usam subjects exclusivos.
const basePath = mchlogtoolkitgo.DebugPath + logger.ServiceName + "/"

func TestMain(m *testing.M) {
	mchlogcore.SetVersion(mchlogcore.V2)
	unittest.RunTests(m)
}

// initRun distingue os subjects entre execuções repetidas do mesmo binário
// (go test -count=N), já que o cache de loggers do pacote é keyed por subject
// e sobrevive ao fim de cada teste.
var initRun int

func TestInitializeMchLog(t *testing.T) {
	initRun++
	subject := func(name string) string { return fmt.Sprintf("init-%s-%d", name, initRun) }

	assert, teardown := unittest.SetupTests(t, subject("absoluto"), subject("barra"), subject("aninhado"))
	defer teardown()

	t.Cleanup(func() { mchlogcorev2.InitializeMchLog(basePath) })

	testCases := []struct {
		name    string
		path    string
		subject string
	}{
		{
			name:    "Inicializa com um caminho absoluto",
			path:    t.TempDir(),
			subject: subject("absoluto"),
		},
		{
			name:    "Inicializa com um caminho terminado em barra",
			path:    t.TempDir() + "/",
			subject: subject("barra"),
		},
		{
			name:    "Cria a árvore de diretórios inexistente",
			path:    t.TempDir() + "/a/b/c",
			subject: subject("aninhado"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			mchlogcorev2.InitializeMchLog(testCase.path)

			assert.Equal(
				filepath.Join(testCase.path, testCase.subject, testCase.subject+".log"),
				mchlogcorev2.MchLog.GetFileNameFromStreamName(testCase.subject),
			)

			mchlogcorev2.MchLog.LogSubject(testCase.subject, map[string]any{"chave": "valor"}, nil)

			unittest.CompareLogs(t, assert, map[string][]string{
				testCase.subject: {`"chave":"valor"`},
			})
		})
	}
}

func TestGetFileNameFromStreamName(t *testing.T) {
	assert, teardown := unittest.SetupTests(t)
	defer teardown()

	testCases := []struct {
		name    string
		subject string
	}{
		{
			name:    "Compõe o nome para um subject simples",
			subject: "teste",
		},
		{
			name:    "Compõe o nome para um subject de erro",
			subject: "err_teste",
		},
		{
			name:    "Compõe o nome para um subject vazio",
			subject: "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			assert.Equal(
				filepath.Join(basePath, testCase.subject, testCase.subject+".log"),
				mchlogcorev2.MchLog.GetFileNameFromStreamName(testCase.subject),
			)
		})
	}
}

func TestLogSubject(t *testing.T) {
	assert, teardown := unittest.SetupTests(t, "teste", "err_teste", "bloqueado")
	defer teardown()

	// Diretório ocupado por um arquivo comum: MkdirAll e OpenFile falham e o
	// pacote apenas reporta o erro, sem gravar log nem entrar em panic.
	blockedPath := filepath.Join(basePath, "bloqueado")
	assert.NoError(os.MkdirAll(basePath, 0755))
	assert.NoError(os.WriteFile(blockedPath, []byte(""), 0644))
	t.Cleanup(func() { _ = os.Remove(blockedPath) })

	testCases := []struct {
		name             string
		subject          string
		contents         []any
		errLog           error
		ascendStackFrame []int
		expectedLogs     map[string][]string
	}{
		{
			name:         "Ignora subject vazio",
			subject:      "",
			contents:     []any{map[string]any{"chave": "valor"}},
			expectedLogs: map[string][]string{},
		},
		{
			name:     "Grava um map",
			subject:  "teste",
			contents: []any{map[string]any{"mapa1": "aaa", "mapa2": 22}},
			expectedLogs: map[string][]string{
				"teste": {`{"mapa1":"aaa","mapa2":22,`},
			},
		},
		{
			name:     "Grava uma string json",
			subject:  "teste",
			contents: []any{`{"testmsg":"some msg"}`},
			expectedLogs: map[string][]string{
				"teste": {`{"testmsg":"some msg",`},
			},
		},
		{
			name:     "Grava um []byte json",
			subject:  "teste",
			contents: []any{[]byte(`{"tick":10}`)},
			expectedLogs: map[string][]string{
				"teste": {`{"tick":10,`},
			},
		},
		{
			name:     "Grava um slice de pares chave/valor",
			subject:  "teste",
			contents: []any{[]any{"key_array1", 22, "key_array2", "55"}},
			expectedLogs: map[string][]string{
				"teste": {`{"key_array1":22,"key_array2":"55",`},
			},
		},
		{
			name:     "Reaproveita o logger em cache em chamadas consecutivas",
			subject:  "teste",
			contents: []any{map[string]any{"seq": 1}, map[string]any{"seq": 2}},
			expectedLogs: map[string][]string{
				"teste": {`{"seq":1,`, `{"seq":2,`},
			},
		},
		{
			name:     "Grava no subject prefixado quando há erro",
			subject:  "teste",
			contents: []any{map[string]any{"chave": "valor"}},
			errLog:   errors.New("isto é um erro forçado"),
			expectedLogs: map[string][]string{
				"err_teste": {`"error":"isto é um erro forçado"`},
			},
		},
		{
			name:             "Respeita o ascendStackFrame informado",
			subject:          "teste",
			contents:         []any{map[string]any{"chave": "valor"}},
			errLog:           errors.New("erro com stack frame"),
			ascendStackFrame: []int{2},
			expectedLogs: map[string][]string{
				"err_teste": {`"error":"erro com stack frame"`},
			},
		},
		{
			name:         "Não grava conteúdo de tipo não suportado",
			subject:      "teste",
			contents:     []any{42},
			expectedLogs: map[string][]string{},
		},
		{
			name:         "Não grava quando o arquivo não pode ser aberto",
			subject:      "bloqueado",
			contents:     []any{map[string]any{"chave": "valor"}},
			expectedLogs: map[string][]string{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			teardownTestCase := unittest.SetupTestCase(t)
			defer teardownTestCase(t)

			for _, content := range testCase.contents {
				mchlogcorev2.MchLog.LogSubject(testCase.subject, content, testCase.errLog, testCase.ascendStackFrame...)
			}

			unittest.CompareLogs(t, assert, testCase.expectedLogs)
		})
	}
}
