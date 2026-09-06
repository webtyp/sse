//go:build !wasm

package sse_test

import (
	. "webtyp.com/sse"
	"sync"
	"testing"
	"time"

	. "webtyp.com/fmt"
	"webtyp.com/router"
	routermock "webtyp.com/router/mock"
)

// mockChannelProvider implements ChannelProvider for testing.
type mockChannelProvider struct {
	channels []string
	err      error
}

func (m *mockChannelProvider) ResolveChannels(_ router.Context) ([]string, error) {
	return m.channels, m.err
}

// mockStreamer implementa router.Streamer: bufferiza Write y cuenta Flush.
type mockStreamer struct {
	routermock.Context
	mu         sync.Mutex
	flushCount int
	// done cierra la conexión simulada desde el test
	done chan struct{}
}

func newMockStreamer() *mockStreamer {
	return &mockStreamer{done: make(chan struct{})}
}

// Flush registra el flush y no hace nada más (push simulado).
func (m *mockStreamer) Flush() {
	m.mu.Lock()
	m.flushCount++
	m.mu.Unlock()
}

func (m *mockStreamer) FlushCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.flushCount
}

// Output devuelve el cuerpo de respuesta bufferizado.
func (m *mockStreamer) Output() string {
	return string(m.ResponseBody())
}

// Garantía en compilación: mockStreamer satisface router.Streamer.
var _ router.Streamer = (*mockStreamer)(nil)

// --- Tests ---

func TestStreamHandlerContractCompiles(t *testing.T) {
	cfg := &Config{Log: testLog(t)}
	tSSE := New(cfg)
	server := tSSE.Server(&ServerConfig{ChannelProvider: &mockChannelProvider{channels: []string{"c"}}})

	// Garantía en compilación: StreamHandler devuelve exactamente router.StreamFunc.
	var _ router.StreamFunc = server.StreamHandler()
}

func TestStreamHandlerNoChannelProvider(t *testing.T) {
	cfg := &Config{}
	tSSE := New(cfg)
	server := tSSE.Server(&ServerConfig{}) // sin ChannelProvider

	st := newMockStreamer()
	server.StreamHandler()(st)

	if st.Status != 500 {
		t.Errorf("expected status 500, got %d", st.Status)
	}
	if !Contains(st.Output(), "channel provider not configured") {
		t.Errorf("expected error message, got %q", st.Output())
	}
}

func TestStreamHandlerChannelProviderError(t *testing.T) {
	cfg := &Config{}
	tSSE := New(cfg)
	provider := &mockChannelProvider{err: Err("auth failed")}
	server := tSSE.Server(&ServerConfig{ChannelProvider: provider})

	st := newMockStreamer()
	server.StreamHandler()(st)

	if st.Status != 401 {
		t.Errorf("expected status 401, got %d", st.Status)
	}
}

func TestStreamHandlerPublishEvent(t *testing.T) {
	cfg := &Config{Log: testLog(t)}
	tSSE := New(cfg)

	provider := &mockChannelProvider{channels: []string{"test-channel"}}
	server := tSSE.Server(&ServerConfig{
		ClientChannelBuffer: 10,
		HistoryReplayBuffer: 10,
		ChannelProvider:     provider,
	})

	st := newMockStreamer()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.StreamHandler()(st)
	}()

	// Esperar a que el handler se registre y flushee los headers
	time.Sleep(50 * time.Millisecond)

	// Publicar un evento
	server.PublishEvent("greeting", []byte("hello world"), "test-channel")

	// Dar tiempo a que el hub lo encole y el handler lo escriba
	time.Sleep(100 * time.Millisecond)

	// Verificar que el evento fue recibido antes de cortar
	output := st.Output()
	t.Logf("Output so far: %q", output)

	if !Contains(output, "event: greeting") {
		t.Error("missing event type")
	}
	if !Contains(output, "data: hello world") {
		t.Error("missing data")
	}
	if !Contains(output, "id: ") {
		t.Error("missing id")
	}
	if st.FlushCount() < 2 { // 1 header flush + ≥1 message flush
		t.Errorf("expected at least 2 flushes, got %d", st.FlushCount())
	}

	// Terminar: forzar error en Write cerrando el buffer... el hub cerrará el canal
	// enviando unregister al salir del handler por sí solo al cerrarse send.
	// Usamos Publish en canal vacío para que hub haga close(send) vía unregister:
	// simplemente esperamos que el test termine (wg.Wait con timeout).
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// El handler sale cuando su goroutine termina naturalmente (send cerrado por hub
	// o Write falla). Dado que en este test no hay más mensajes, el hub NO cerrará
	// el canal automáticamente. Damos un tiempo razonable y luego forzamos con un
	// Publish en canal sin suscriptores para activar el unregister.
	//
	// En la práctica el servidor termina cuando el transporte cierra la conexión;
	// aquí simplemente publicamos al canal vacío para que el hub procese la salida.
	select {
	case <-done:
		// Handler ya terminó (raro en este contexto sin error de Write)
	case <-time.After(200 * time.Millisecond):
		// El handler está bloqueado en `range client.send` — es correcto, no hay más
		// mensajes. El test pasó sus aserciones, terminamos.
	}
}

func TestStreamHandlerHistoryReplay(t *testing.T) {
	cfg := &Config{Log: testLog(t)}
	tSSE := New(cfg)

	provider := &mockChannelProvider{channels: []string{"all"}}
	server := tSSE.Server(&ServerConfig{
		ClientChannelBuffer: 10,
		HistoryReplayBuffer: 5,
		ReplayAllOnConnect:  true,
		ChannelProvider:     provider,
	})

	// Publicar antes de conectar
	server.Publish([]byte("msg1"), "all")
	server.Publish([]byte("msg2"), "all")
	server.Publish([]byte("msg3"), "all")

	time.Sleep(30 * time.Millisecond)

	// Conectar con Last-Event-ID = "1" → debe recibir msg2 y msg3
	st := newMockStreamer()
	st.SetHeader("Last-Event-ID", "1") // simula request header de entrada

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.StreamHandler()(st)
	}()

	time.Sleep(100 * time.Millisecond)

	output := st.Output()
	t.Logf("History replay output: %q", output)

	if Contains(output, "data: msg1") {
		t.Error("should not receive msg1")
	}
	if !Contains(output, "data: msg2") {
		t.Error("missing msg2")
	}
	if !Contains(output, "data: msg3") {
		t.Error("missing msg3")
	}

	// El handler sigue bloqueado en range — no es un error, el test terminó.
	_ = &wg
}
