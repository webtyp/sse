---
PLAN: "fix(sse): a stream ends when its client disconnects, not on the next failed write"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 17567039206527582536
PR: https://github.com/webtyp/sse/pull/7
---

> Este plan se despacha con el flujo CodeJob. Ver skill: agents-workflow.
> Lee `AGENTS.md` en la raíz primero.
> Orden de la ola: 1. `webtyp/router` v0.2.0 → 2. `webtyp/server` (httpd implementa `Done`) →
> **3. `webtyp/sse` (este)**. **Espera los tags de los dos anteriores.**

# Plan — el stream termina cuando el cliente se va

## 0. El defecto (verificado)

`server.go`, `streamHandler`, paso 5:
```go
for msg := range client.send {
	if _, err := st.Write(msg); err != nil { return }
	st.Flush()
}
```
El handler solo retorna cuando una escritura falla, y eso solo pasa cuando llega un mensaje a uno de sus
canales. Con canales por usuario (`mjosefa-cms` usa `chat_room.inbox.<userID>`), una pestaña cerrada deja
viva su gorutina y su `clientConnection` registrada en el hub hasta el próximo aviso a ese usuario — o para
siempre. Y `httptest.Server.Close()` se bloquea en cualquier test que abra un stream: así se detectó.

`webtyp.com/router` v0.2.0 agrega `router.Streamer.Done() <-chan struct{}` (se cierra al desconectarse el
cliente); `webtyp.com/server` lo implementa en `httpd` con el contexto de la petición.

## Etapas

1. `go get webtyp.com/router@v0.2.0 webtyp.com/server@v0.2.62` + `go mod tidy`.
2. **Test rojo primero** — `tests/disconnect_test.go` (`//go:build !wasm`): servidor real
   (`httptest.NewServer` + router de `webtyp.com/server/httpd`) con `SSEServer.MountAPI`, `Access: model.AccessPublic`,
   un `ChannelProvider` que devuelve `[]string{"c1"}`. Cliente real con `context.WithCancel`: `GET` al path,
   leer las cabeceras, **cancelar**. Aserciones:
   - `ts.Close()` retorna en menos de 2 s (ejecutarlo en una gorutina y `select` con `time.After`);
   - el hub ya no tiene clientes: exponer lo mínimo para verlo **solo si no existe ya** una forma — preferir
     una aserción indirecta: después de cancelar, `PublishEvent("x", []byte("y"), "c1")` no debe bloquear ni
     encontrar destinatarios (si no hay forma limpia de observarlo sin exportar internals, quedarse solo con
     la aserción de `ts.Close()`).
   Hoy el test falla por timeout. Confirmarlo antes de cambiar `server.go`.
3. `server.go`, paso 5:
   ```go
   for {
   	select {
   	case <-st.Done():
   		return // client gone: the deferred unregister runs
   	case msg, ok := <-client.send:
   		if !ok {
   			return
   		}
   		if _, err := st.Write(msg); err != nil {
   			return
   		}
   		st.Flush()
   	}
   }
   ```
4. `tests/test_router_test.go`: `mockStreamer` implementa `Done()` (un canal que el test controla; en los
   tests existentes basta uno que nunca se cierra).
5. `docs/USAGE.md`: una línea en "Server-Side": el stream se cierra solo cuando el navegador se va.

## Criterios de aceptación

```bash
gotest                                   # verde, incluido tests/disconnect_test.go
grep -n "st.Done()" server.go            # → 1
```

## Después (fuera de este plan)

`mjosefa-cms` recupera su test del stream autenticado (`GET /inbox` con sesión → 200 +
`text/event-stream`), que se quitó del PR de integración porque no podía cerrar limpio.
