package mchlogcorev2

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_assert "github.com/stretchr/testify/assert"
)

// Os testes NÃO usam t.Parallel: MchLog e o cache mapLogger são globais do
// pacote e seriam compartilhados entre casos paralelos.

// subjectSeq garante subjects únicos no binário de teste. O cache mapLogger é
// keyed por subject e segura o arquivo aberto durante toda a vida do processo,
// então reusar um subject faria a gravação cair no diretório do primeiro caso.
var subjectSeq int

func uniqueSubject(prefix string) string {
	subjectSeq++
	return fmt.Sprintf("%s-%d", prefix, subjectSeq)
}

// readLogLines devolve as linhas gravadas no arquivo de log, ou nil quando
// nada foi gravado — seja porque o arquivo não existe, seja porque o caminho
// sequer é percorrível (o caso do subject bloqueado por um arquivo comum).
func readLogLines(t *testing.T, fileName string) []string {
	t.Helper()

	content, err := os.ReadFile(fileName)
	if err != nil {
		return nil
	}

	trimmed := strings.TrimRight(string(content), "\n")
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}

func TestInitializeMchLog(t *testing.T) {
	assert := _assert.New(t)

	testCases := []struct {
		name string
		path string
	}{
		{
			name: "Inicializa com um caminho absoluto",
			path: t.TempDir(),
		},
		{
			name: "Inicializa com um caminho terminado em barra",
			path: t.TempDir() + "/",
		},
		{
			name: "Cria a árvore de diretórios inexistente",
			path: t.TempDir() + "/a/b/c",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			InitializeMchLog(testCase.path)

			assert.Equal(filepath.FromSlash(testCase.path), MchLog.path)

			subject := uniqueSubject("init")
			MchLog.LogSubject(subject, map[string]any{"chave": "valor"}, nil)

			lines := readLogLines(t, filepath.Join(testCase.path, subject, subject+ccLogFileSuffix))
			assert.Len(lines, 1)
			assert.Contains(lines[0], `"chave":"valor"`)
		})
	}
}

func TestGetFileNameFromStreamName(t *testing.T) {
	assert := _assert.New(t)

	basePath := t.TempDir()
	InitializeMchLog(basePath)

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
			subject: ccLogErrPrefixSubject + "teste",
		},
		{
			name:    "Compõe o nome para um subject vazio",
			subject: "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(
				filepath.Join(basePath, testCase.subject, testCase.subject+ccLogFileSuffix),
				MchLog.GetFileNameFromStreamName(testCase.subject),
			)
		})
	}
}

func TestLogSubject(t *testing.T) {
	assert := _assert.New(t)

	basePath := t.TempDir()
	InitializeMchLog(basePath)

	// Diretório ocupado por um arquivo comum: MkdirAll e OpenFile falham e o
	// pacote apenas reporta o erro, sem gravar log nem entrar em panic.
	blocked := uniqueSubject("bloqueado")
	assert.NoError(os.WriteFile(filepath.Join(basePath, blocked), []byte(""), 0644))

	testCases := []struct {
		name             string
		emptySubject     bool
		blockedSubject   bool
		contents         []any
		errLog           error
		ascendStackFrame []int
		expectedLines    []string
	}{
		{
			name:         "Ignora subject vazio",
			emptySubject: true,
			contents:     []any{map[string]any{"chave": "valor"}},
		},
		{
			name:          "Grava um map",
			contents:      []any{map[string]any{"mapa1": "aaa", "mapa2": 22}},
			expectedLines: []string{`{"mapa1":"aaa","mapa2":22,`},
		},
		{
			name:          "Grava uma string json",
			contents:      []any{`{"testmsg":"some msg"}`},
			expectedLines: []string{`{"testmsg":"some msg",`},
		},
		{
			name:          "Grava um []byte json",
			contents:      []any{[]byte(`{"tick":10}`)},
			expectedLines: []string{`{"tick":10,`},
		},
		{
			name:          "Grava um slice de pares chave/valor",
			contents:      []any{[]any{"key_array1", 22, "key_array2", "55"}},
			expectedLines: []string{`{"key_array1":22,"key_array2":"55",`},
		},
		{
			name:          "Reaproveita o logger em cache em chamadas consecutivas",
			contents:      []any{map[string]any{"seq": 1}, map[string]any{"seq": 2}},
			expectedLines: []string{`{"seq":1,`, `{"seq":2,`},
		},
		{
			name:          "Grava no subject prefixado quando há erro",
			contents:      []any{map[string]any{"chave": "valor"}},
			errLog:        errors.New("isto é um erro forçado"),
			expectedLines: []string{`"error":"isto é um erro forçado"`},
		},
		{
			name:             "Respeita o ascendStackFrame informado",
			contents:         []any{map[string]any{"chave": "valor"}},
			errLog:           errors.New("erro com stack frame"),
			ascendStackFrame: []int{2},
			expectedLines:    []string{`"error":"erro com stack frame"`},
		},
		{
			name:     "Não grava conteúdo de tipo não suportado",
			contents: []any{42},
		},
		{
			name:           "Não grava quando o arquivo não pode ser aberto",
			blockedSubject: true,
			contents:       []any{map[string]any{"chave": "valor"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			subject := uniqueSubject("log")
			switch {
			case testCase.emptySubject:
				subject = ""
			case testCase.blockedSubject:
				subject = blocked
			}

			for _, content := range testCase.contents {
				MchLog.LogSubject(subject, content, testCase.errLog, testCase.ascendStackFrame...)
			}

			fileSubject := subject
			if testCase.errLog != nil {
				fileSubject = ccLogErrPrefixSubject + subject
			}

			lines := readLogLines(t, filepath.Join(basePath, fileSubject, fileSubject+ccLogFileSuffix))
			assert.Len(lines, len(testCase.expectedLines))

			for i, expected := range testCase.expectedLines {
				assert.Contains(lines[i], expected)
				assert.Contains(lines[i], `"`+ccLogDataHora+`":"`+time.Now().UTC().Format("2006-01-02 15:04:05"))
			}
		})
	}
}
