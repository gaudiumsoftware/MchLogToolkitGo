package mchlogcorev3

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// currentGraylogUDP devolve o *graylogUDP atualmente embrulhado pelo
// routerDestination ativo. Falha o teste com t.Fatalf se MchLog não
// estiver inicializado, se o impl não for um router, ou se o router
// não tiver um network impl do tipo *graylogUDP.
//
// Helper compartilhado por failure_test.go e mchlogv3_test.go, que
// antes acessavam MchLog.impl.(*graylogUDP) diretamente. Como o V3
// agora sempre embrulha em router, esse atalho não funciona mais.
func currentGraylogUDP(t *testing.T) *graylogUDP {
	t.Helper()
	MchLog.mu.RLock()
	impl := MchLog.impl
	MchLog.mu.RUnlock()
	r, ok := impl.(*routerDestination)
	if !ok {
		t.Fatalf("expected *routerDestination, got %T", impl)
	}
	g, ok := r.network.(*graylogUDP)
	if !ok {
		t.Fatalf("expected router.network to be *graylogUDP, got %T", r.network)
	}
	return g
}

// recordingDestination é um destination de teste que apenas anota a
// chamada e devolve respostas pré-programadas.
type recordingDestination struct {
	name string

	mu          sync.Mutex
	logCalls    []string // subjects recebidos
	getCalls    []string
	closeCalls  int
	closeErr    error
	descriptor  string // valor devolvido por GetFileNameFromStreamName
}

func newRecorder(name, descriptor string) *recordingDestination {
	return &recordingDestination{name: name, descriptor: descriptor}
}

func (d *recordingDestination) LogSubject(subject string, _ any, _ error, _ ...int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logCalls = append(d.logCalls, subject)
}

func (d *recordingDestination) GetFileNameFromStreamName(subject string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.getCalls = append(d.getCalls, subject)
	return d.descriptor + ":" + subject
}

func (d *recordingDestination) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closeCalls++
	return d.closeErr
}

func (d *recordingDestination) logged() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.logCalls))
	copy(out, d.logCalls)
	return out
}

func (d *recordingDestination) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logCalls = nil
	d.getCalls = nil
}

// TestRouterDispatch cobre a matriz de roteamento da SPEC §5.1.
func TestRouterDispatch(t *testing.T) {
	cases := []struct {
		name      string
		subject   string
		hasNet    bool
		extra     []string
		wantWhere string // "network" | "file" | "noop"
	}{
		{name: "leveled info → network", subject: "info", hasNet: true, wantWhere: "network"},
		{name: "leveled debug → network", subject: "debug", hasNet: true, wantWhere: "network"},
		{name: "leveled warn → network", subject: "warn", hasNet: true, wantWhere: "network"},
		{name: "leveled error → network", subject: "error", hasNet: true, wantWhere: "network"},
		{name: "leveled fatal → network", subject: "fatal", hasNet: true, wantWhere: "network"},
		{name: "leveled test → network", subject: "test", hasNet: true, wantWhere: "network"},
		{name: "domain subject → file", subject: "historico_posicao_taxi", hasNet: true, wantWhere: "file"},
		{name: "extender adds domain → network", subject: "historico_posicao_taxi", hasNet: true, extra: []string{"historico_posicao_taxi"}, wantWhere: "network"},
		{name: "extender empty entry ignored", subject: "", hasNet: true, extra: []string{""}, wantWhere: "noop"},
		{name: "leveled info, no network → file", subject: "info", hasNet: false, wantWhere: "file"},
		{name: "domain subject, no network → file", subject: "historico_posicao_taxi", hasNet: false, wantWhere: "file"},
		{name: "empty subject → noop", subject: "", hasNet: true, wantWhere: "noop"},
		{name: "empty subject, no network → noop", subject: "", hasNet: false, wantWhere: "noop"},
		{name: "case-sensitive: INFO not in whitelist → file", subject: "INFO", hasNet: true, wantWhere: "file"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := newRecorder("file", "file")
			var network destination
			if tc.hasNet {
				network = newRecorder("net", "udp")
			}
			r := newRouterDestination(file, network, tc.extra)

			r.LogSubject(tc.subject, []byte(`{"message":"x"}`), nil)

			fileGot := file.logged()
			var netGot []string
			if rec, ok := network.(*recordingDestination); ok {
				netGot = rec.logged()
			}

			switch tc.wantWhere {
			case "network":
				if len(netGot) != 1 || netGot[0] != tc.subject {
					t.Errorf("network impl got %v, want [%q]", netGot, tc.subject)
				}
				if len(fileGot) != 0 {
					t.Errorf("file impl got %v, want empty", fileGot)
				}
			case "file":
				if len(fileGot) != 1 || fileGot[0] != tc.subject {
					t.Errorf("file impl got %v, want [%q]", fileGot, tc.subject)
				}
				if len(netGot) != 0 {
					t.Errorf("network impl got %v, want empty", netGot)
				}
			case "noop":
				if len(fileGot) != 0 || len(netGot) != 0 {
					t.Errorf("expected no calls, got file=%v network=%v", fileGot, netGot)
				}
			}
		})
	}
}

// TestRouterGetFileNameFromStreamNameMatrix garante que o descritor é
// resolvido pelo impl que receberia o LogSubject.
func TestRouterGetFileNameFromStreamNameMatrix(t *testing.T) {
	file := newRecorder("file", "file")
	network := newRecorder("net", "udp")
	r := newRouterDestination(file, network, []string{"historico_posicao_taxi"})

	cases := map[string]string{
		"info":                   "udp:info",
		"historico_posicao_taxi": "udp:historico_posicao_taxi",
		"log_posicao_alterada":   "file:log_posicao_alterada",
		"":                       "",
	}
	for subj, want := range cases {
		if got := r.GetFileNameFromStreamName(subj); got != want {
			t.Errorf("GetFileNameFromStreamName(%q) = %q want %q", subj, got, want)
		}
	}

	// Quando network é nil, todos os subjects resolvem via file.
	r2 := newRouterDestination(file, nil, nil)
	if got := r2.GetFileNameFromStreamName("info"); got != "file:info" {
		t.Errorf("router without network: GetFileNameFromStreamName(info) = %q want file:info", got)
	}
}

// TestRouterCloseBoth garante que Close fecha file e network exatamente
// uma vez, independente de erros.
func TestRouterCloseBoth(t *testing.T) {
	file := newRecorder("file", "file")
	network := newRecorder("net", "udp")
	network.closeErr = errors.New("net failed")

	r := newRouterDestination(file, network, nil)
	if err := r.Close(); err == nil {
		t.Fatalf("expected non-nil error from Close (network failure should propagate)")
	}
	if file.closeCalls != 1 {
		t.Errorf("file.Close calls = %d want 1", file.closeCalls)
	}
	if network.closeCalls != 1 {
		t.Errorf("network.Close calls = %d want 1", network.closeCalls)
	}
}

// TestRouterCloseNetworkNil garante que Close funciona quando não há
// network configurado.
func TestRouterCloseNetworkNil(t *testing.T) {
	file := newRecorder("file", "file")
	r := newRouterDestination(file, nil, nil)
	if err := r.Close(); err != nil {
		t.Errorf("Close = %v want nil", err)
	}
	if file.closeCalls != 1 {
		t.Errorf("file.Close calls = %d want 1", file.closeCalls)
	}
}

// TestRouterCloseNilReceiver garante que Close em receiver nil é no-op.
func TestRouterCloseNilReceiver(t *testing.T) {
	var r *routerDestination
	if err := r.Close(); err != nil {
		t.Errorf("nil Close = %v want nil", err)
	}
}

// TestRouterLogSubjectNilReceiver garante que LogSubject em receiver
// nil não panica (defensivo; o facade já tem outro guard).
func TestRouterLogSubjectNilReceiver(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil receiver panicked: %v", r)
		}
	}()
	var r *routerDestination
	r.LogSubject("info", nil, nil)
	if got := r.GetFileNameFromStreamName("info"); got != "" {
		t.Errorf("nil GetFileNameFromStreamName = %q want empty", got)
	}
}

// TestEndToEndLeveledHitsUDPNotFile dispara LogSubject("info", …) com
// network configurado e verifica que: (1) datagrama chega ao listener
// UDP; (2) o arquivo .../info/info.log NÃO é criado em disco.
func TestEndToEndLeveledHitsUDPNotFile(t *testing.T) {
	t.Cleanup(resetConfig)

	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Network: &NetworkConfig{
		Type:   NetworkGraylogUDP,
		Addr:   addr,
		Source: "pod-1",
	}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	dir := t.TempDir()
	servicePath := filepath.Join(dir, "svc") + string(filepath.Separator)
	if err := Initialize(servicePath); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	payload := []byte(`{"message":"hello","level":"info","source":"x.go","line":"1","trace":""}`)
	MchLog.LogSubject("info", payload, nil)

	raw := readDatagram(t, conn)
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("invalid GELF JSON: %v", err)
	}
	if got["short_message"] != "hello" {
		t.Errorf("short_message=%v want hello", got["short_message"])
	}

	filePath := filepath.Join(dir, "svc", "info", "info.log")
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("info.log should NOT have been created at %q (err=%v)", filePath, err)
	}
}

// TestEndToEndDomainHitsFileNotUDP dispara LogSubject("historico_posicao_taxi", …)
// com network configurado e verifica que: (1) o arquivo é criado em
// disco com a shape do V2; (2) NÃO chega datagrama no listener UDP.
func TestEndToEndDomainHitsFileNotUDP(t *testing.T) {
	t.Cleanup(resetConfig)

	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Network: &NetworkConfig{
		Type:   NetworkGraylogUDP,
		Addr:   addr,
		Source: "pod-1",
	}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	dir := t.TempDir()
	servicePath := filepath.Join(dir, "svc") + string(filepath.Separator)
	if err := Initialize(servicePath); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	payload := []byte(`{"message":"pos_event","level":"info","source":"x.go","line":"1","trace":""}`)
	MchLog.LogSubject("historico_posicao_taxi", payload, nil)

	expectNoDatagram(t, conn, 200*time.Millisecond)

	filePath := filepath.Join(dir, "svc", "historico_posicao_taxi", "historico_posicao_taxi.log")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("expected file at %q: %v", filePath, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("no log lines written to %q", filePath)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &got); err != nil {
		t.Fatalf("invalid JSON line: %v\nline=%s", err, lines[len(lines)-1])
	}
	if got["message"] != "pos_event" {
		t.Errorf("file content: message=%v want pos_event", got["message"])
	}
}

// TestEndToEndNetworkSubjectsExtender garante que um subject de domínio
// listado em NetworkSubjects é roteado ao UDP em vez de ao disco.
func TestEndToEndNetworkSubjectsExtender(t *testing.T) {
	t.Cleanup(resetConfig)

	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{
		Network: &NetworkConfig{
			Type:   NetworkGraylogUDP,
			Addr:   addr,
			Source: "pod-1",
		},
		NetworkSubjects: []string{"historico_posicao_taxi"},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	dir := t.TempDir()
	servicePath := filepath.Join(dir, "svc") + string(filepath.Separator)
	if err := Initialize(servicePath); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	payload := []byte(`{"message":"pos","level":"info","source":"x.go","line":"1","trace":""}`)
	MchLog.LogSubject("historico_posicao_taxi", payload, nil)

	raw := readDatagram(t, conn)
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("invalid GELF JSON: %v", err)
	}
	if got["short_message"] != "pos" {
		t.Errorf("short_message=%v want pos", got["short_message"])
	}

	filePath := filepath.Join(dir, "svc", "historico_posicao_taxi", "historico_posicao_taxi.log")
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("extender subject should not produce file at %q (err=%v)", filePath, err)
	}
}

// TestFacadeGetFileNameFromStreamNameRouting confirma que o facade
// expõe a regra do router: leveled → udp://…; domain → file path.
func TestFacadeGetFileNameFromStreamNameRouting(t *testing.T) {
	t.Cleanup(resetConfig)

	addr, conn := listenUDP(t)
	defer conn.Close()

	if err := Configure(DestinationConfig{Network: &NetworkConfig{
		Type:   NetworkGraylogUDP,
		Addr:   addr,
		Source: "pod-1",
	}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	dir := t.TempDir()
	servicePath := filepath.Join(dir, "svc") + string(filepath.Separator)
	if err := Initialize(servicePath); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	if got, want := MchLog.GetFileNameFromStreamName("info"), "udp://"+addr+"/info"; got != want {
		t.Errorf("leveled descriptor = %q want %q", got, want)
	}
	if got, want := MchLog.GetFileNameFromStreamName("historico_posicao_taxi"),
		filepath.Join(dir, "svc", "historico_posicao_taxi", "historico_posicao_taxi.log"); got != want {
		t.Errorf("domain descriptor = %q want %q", got, want)
	}
}

// TestRouterConcurrentLogSubject roda muitas goroutines escrevendo via
// router (leveled+domain) e exige (a) zero races; (b) datagramas
// recebidos = N_leveled; (c) linhas no arquivo de domínio = N_domain.
func TestRouterConcurrentLogSubject(t *testing.T) {
	t.Cleanup(resetConfig)

	addr, conn := listenUDP(t)
	defer conn.Close()
	// Buffer maior para evitar drops sob carga em loopback.
	if u, ok := conn.(*net.UDPConn); ok {
		_ = u.SetReadBuffer(1 << 20)
	}

	if err := Configure(DestinationConfig{Network: &NetworkConfig{
		Type:   NetworkGraylogUDP,
		Addr:   addr,
		Source: "pod-1",
	}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	dir := t.TempDir()
	servicePath := filepath.Join(dir, "svc") + string(filepath.Separator)
	if err := Initialize(servicePath); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = MchLog.Close() })

	const goroutines = 20
	const perGoroutine = 20
	// Subject único deste teste para não colidir com o cache global do V2
	// (mchlogcorev2 cacheia um *zerolog.Logger por subject, com referência
	// ao FD; se outro teste usar o mesmo subject em tempdir diferente, o
	// cache aponta para o FD antigo, que ficou unlinked).
	const domainSubject = "concurrent_router_domain_subject"

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				MchLog.LogSubject("info", []byte(`{"message":"x"}`), nil)
				MchLog.LogSubject(domainSubject, []byte(`{"message":"x"}`), nil)
			}
		}()
	}
	wg.Wait()

	// Drena datagramas UDP por até 1s. Loopback pode dropar sob
	// rajada; aceitamos qualquer N > 0 como sinal de que rota network
	// está viva, e cobrimos a contagem exata via inspeção de arquivo.
	gotDatagrams := 0
	_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 64*1024)
	for {
		if _, _, err := conn.ReadFrom(buf); err != nil {
			break
		}
		gotDatagrams++
	}
	if gotDatagrams == 0 {
		t.Errorf("expected at least 1 UDP datagram for leveled subjects, got 0")
	}

	filePath := filepath.Join(dir, "svc", domainSubject, domainSubject+".log")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("expected file at %q: %v", filePath, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := goroutines * perGoroutine
	if len(lines) != want {
		t.Errorf("file line count = %d want %d", len(lines), want)
	}
}

// expectNoDatagram verifica que nenhum datagrama chega à conn dentro
// do timeout. Helper local para evitar duplicar a lógica.
func expectNoDatagram(t *testing.T, conn net.PacketConn, timeout time.Duration) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 4096)
	if n, _, err := conn.ReadFrom(buf); err == nil {
		t.Fatalf("unexpected datagram (%d bytes): %s", n, string(buf[:n]))
	}
}
