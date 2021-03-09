package main

import (
	"net/http"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	clients   = make(map[string]*Client)
	clientsEx sync.Mutex
)

func DispatchEvent(payload EventPayload) {
	clientsEx.Lock()
	for _, client := range clients {
		client.SendEvent(payload)
	}
	clientsEx.Unlock()
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Warn("upgrade:", err)
		return
	}
	defer c.Close()

	client := NewClient(c, r)
	clientsEx.Lock()
	clients[client.GUID] = client
	clientsEx.Unlock()
	defer func() {
		clientsEx.Lock()
		delete(clients, client.GUID)
		clientsEx.Unlock()
	}()

	for {
		mt, data, err := c.ReadMessage()
		if err != nil {
			log.Warnf("read: %v", err)
			return
		}
		switch mt {
		case websocket.BinaryMessage:
			if err = client.handleBinary(data); err != nil {
				log.Warnf("handle binary: %v", err)
				return
			}
		default:
			log.Warnf("unknown websocket message type: %v", mt)
			return
		}
	}
}
