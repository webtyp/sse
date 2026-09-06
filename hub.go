//go:build !wasm

package sse

import (
	"bytes"
	"sync"

	. "webtyp.com/fmt"
)

// hub manages SSE clients and broadcasting.
type hub struct {
	tinySSE *tinySSE
	config  *ServerConfig

	// Registered clients.
	clients map[*clientConnection]bool

	// Inbound messages from the clients.
	broadcast chan *broadcastMessage

	// Register requests from the clients.
	register chan registerRequest

	// Unregister requests from clients.
	unregister chan *clientConnection

	// History buffer
	history      []*historyItem
	historyMutex sync.RWMutex
	lastID       int
}

type registerRequest struct {
	client      *clientConnection
	lastEventID string
}

type broadcastMessage struct {
	msg      *SSEMessage
	channels []string
}

type historyItem struct {
	msg      *SSEMessage
	channels []string
}

// clientConnection represents a connected SSE client on the server side.
type clientConnection struct {
	channels []string
	send     chan []byte
}

func newHub(t *tinySSE, c *ServerConfig) *hub {
	h := &hub{
		tinySSE:    t,
		config:     c,
		broadcast:  make(chan *broadcastMessage),
		register:   make(chan registerRequest),
		unregister: make(chan *clientConnection),
		clients:    make(map[*clientConnection]bool),
		history:    make([]*historyItem, 0, c.HistoryReplayBuffer),
	}
	go h.run()
	return h
}

func (h *hub) run() {
	for {
		select {
		case req := <-h.register:
			h.clients[req.client] = true
			h.replayHistory(req.client, req.lastEventID)

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}

		case bMsg := <-h.broadcast:
			// 1. Assign ID
			bMsg.msg.Id = h.nextID()

			// 2. Add to history
			h.addToHistory(bMsg.msg, bMsg.channels)

			// 3. Format message once
			formattedMsg := formatSSEMessage(bMsg.msg.Id, bMsg.msg.Event, bMsg.msg.Data)
			dataBytes := []byte(formattedMsg)

			// 4. Send to interested clients
			for client := range h.clients {
				if h.isSubscribed(client, bMsg.channels) {
					select {
					case client.send <- dataBytes:
					default:
						h.tinySSE.log("Dropping message for slow client")
					}
				}
			}
		}
	}
}

func (h *hub) nextID() string {
	h.lastID++
	return Convert(h.lastID).String()
}

func (h *hub) addToHistory(msg *SSEMessage, channels []string) {
	if h.config.HistoryReplayBuffer <= 0 {
		return
	}
	h.historyMutex.Lock()
	defer h.historyMutex.Unlock()

	item := &historyItem{
		msg:      msg,
		channels: channels,
	}

	h.history = append(h.history, item)
	if len(h.history) > h.config.HistoryReplayBuffer {
		h.history = h.history[1:] // Remove oldest
	}
}

func (h *hub) replayHistory(client *clientConnection, lastEventID string) {
	if h.config.HistoryReplayBuffer <= 0 {
		return
	}

	// No Last-Event-ID: replay all history if ReplayAllOnConnect is enabled
	if lastEventID == "" {
		if !h.config.ReplayAllOnConnect {
			return
		}
		h.historyMutex.RLock()
		defer h.historyMutex.RUnlock()
		for _, item := range h.history {
			if h.isSubscribed(client, item.channels) {
				formattedMsg := formatSSEMessage(item.msg.Id, item.msg.Event, item.msg.Data)
				client.send <- []byte(formattedMsg)
			}
		}
		return
	}

	h.historyMutex.RLock()
	defer h.historyMutex.RUnlock()

	// Find where to start (after the last known event ID)
	startIndex := -1
	for i, item := range h.history {
		if item.msg.Id == lastEventID {
			startIndex = i + 1
			break
		}
	}

	if startIndex != -1 && startIndex < len(h.history) {
		for i := startIndex; i < len(h.history); i++ {
			item := h.history[i]
			if h.isSubscribed(client, item.channels) {
				formattedMsg := formatSSEMessage(item.msg.Id, item.msg.Event, item.msg.Data)
				client.send <- []byte(formattedMsg)
			}
		}
	}
}

func (h *hub) isSubscribed(client *clientConnection, messageChannels []string) bool {
	if len(messageChannels) == 0 {
		return false
	}

	for _, msgChan := range messageChannels {
		for _, clientChan := range client.channels {
			if msgChan == clientChan {
				return true
			}
		}
	}
	return false
}

func formatSSEMessage(id, event string, data []byte) string {
	var b bytes.Buffer
	b.WriteString("id: ")
	b.WriteString(id)
	b.WriteString("\n")

	if event != "" {
		b.WriteString("event: ")
		b.WriteString(event)
		b.WriteString("\n")
	}

	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSuffix(line, []byte("\r"))
		b.WriteString("data: ")
		b.Write(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	return b.String()
}
