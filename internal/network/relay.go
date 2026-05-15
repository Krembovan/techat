package network

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/Krembovan/techat/internal/model"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

type ClientSession struct {
	conn     *websocket.Conn
	username string
	pubKey   string
	sendCh   chan []byte
}

type Relay struct {
	addr    string
	clients map[string]*ClientSession
	mu      sync.RWMutex
}

func NewRelay(addr string) *Relay {
	return &Relay{
		addr:    addr,
		clients: make(map[string]*ClientSession),
	}
}

func (r *Relay) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", r.handleWebSocket)
	mux.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	server := &http.Server{
		Addr:         r.addr,
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	log.Printf("Relay server starting on %s", r.addr)
	return server.ListenAndServe()
}

func (r *Relay) handleWebSocket(w http.ResponseWriter, req *http.Request) {
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	session := &ClientSession{
		conn:   conn,
		sendCh: make(chan []byte, 256),
	}

	go r.writePump(session)
	r.readPump(session)
}

func (r *Relay) readPump(session *ClientSession) {
	defer func() {
		r.unregisterClient(session)
		session.conn.Close()
	}()

	session.conn.SetReadLimit(10 * 1024 * 1024) // 10MB max message
	session.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	session.conn.SetPongHandler(func(string) error {
		session.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, data, err := session.conn.ReadMessage()
		if err != nil {
			break
		}

		var msg model.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		msg.Timestamp = time.Now().UnixMilli()

		switch msg.Type {
		case model.MsgRegister:
			session.username = msg.Sender
			session.pubKey = msg.PubKey
			r.registerClient(session)
			r.broadcastPresence(session, "online")

		case model.MsgGlobal:
			msg.Sender = session.username
			r.broadcastGlobal(msg, session.username)

		case model.MsgDirect:
			msg.Sender = session.username
			r.routeToUser(msg)

		case model.MsgTyping:
			msg.Sender = session.username
			r.routeToUser(msg)

		case model.MsgFileOffer:
			msg.Sender = session.username
			r.routeToUser(msg)

		case model.MsgFileChunk:
			msg.Sender = session.username
			r.routeToUser(msg)

		case model.MsgRead:
			msg.Sender = session.username
			r.routeToUser(msg)

		case model.MsgKeyExchange:
			msg.Sender = session.username
			r.routeToUser(msg)
		}
	}
}

func (r *Relay) writePump(session *ClientSession) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		session.conn.Close()
	}()

	for {
		select {
		case data, ok := <-session.sendCh:
			if !ok {
				return
			}
			session.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := session.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			session.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := session.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (r *Relay) registerClient(session *ClientSession) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if old, ok := r.clients[session.username]; ok {
		close(old.sendCh)
		old.conn.Close()
	}
	r.clients[session.username] = session
	log.Printf("User connected: %s (total: %d)", session.username, len(r.clients))
}

func (r *Relay) unregisterClient(session *ClientSession) {
	r.mu.Lock()
	if existing, ok := r.clients[session.username]; ok && existing == session {
		delete(r.clients, session.username)
		log.Printf("User disconnected: %s (total: %d)", session.username, len(r.clients))
	}
	r.mu.Unlock()

	offlineMsg := model.Message{
		Type:   model.MsgPresence,
		Sender: session.username,
		Status: "offline",
	}
	data, _ := json.Marshal(offlineMsg)

	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, client := range r.clients {
		select {
		case client.sendCh <- data:
		default:
		}
	}
}

func (r *Relay) broadcastPresence(session *ClientSession, status string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	type userEntry struct {
		Username string `json:"username"`
		PubKey   string `json:"pubkey"`
	}
	var userList []userEntry
	for _, client := range r.clients {
		userList = append(userList, userEntry{
			Username: client.username,
			PubKey:   client.pubKey,
		})
	}
	listData, _ := json.Marshal(userList)

	presenceData, _ := json.Marshal(model.Message{
		Type:    model.MsgPresence,
		Status:  "userlist",
		Content: string(listData),
	})
	select {
	case session.sendCh <- presenceData:
	default:
	}

	joinMsg := model.Message{
		Type:   model.MsgPresence,
		Sender: session.username,
		PubKey: session.pubKey,
		Status: "online",
	}
	data, _ := json.Marshal(joinMsg)
	for _, client := range r.clients {
		if client != session {
			select {
			case client.sendCh <- data:
			default:
			}
		}
	}
}

func (r *Relay) broadcastGlobal(msg model.Message, excludeUser string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	data, _ := json.Marshal(msg)
	for username, client := range r.clients {
		if username != excludeUser {
			select {
			case client.sendCh <- data:
			default:
			}
		}
	}
}

func (r *Relay) routeToUser(msg model.Message) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	data, _ := json.Marshal(msg)
	if client, ok := r.clients[msg.Recipient]; ok {
		select {
		case client.sendCh <- data:
		default:
			if sender, ok := r.clients[msg.Sender]; ok {
				errMsg, _ := json.Marshal(model.Message{
					Type:    model.MsgError,
					Content: "recipient buffer full, message not delivered",
				})
				sender.sendCh <- errMsg
			}
		}
	} else {
		if sender, ok := r.clients[msg.Sender]; ok {
			errMsg, _ := json.Marshal(model.Message{
				Type:    model.MsgError,
				Content: "user is offline or not found",
			})
			sender.sendCh <- errMsg
		}
	}
}
