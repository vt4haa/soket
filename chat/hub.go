package chat

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Message - структура сообщения в чате
type Message struct {
	Room      string `json:"room"`
	Username  string `json:"username"`
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
}

// Repository - интерфейс для работы с хранилищем сообщений
type Repository interface {
	SaveMessage(msg Message) error
	GetLastMessages(room string, count int) ([]Message, error)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Client - представляет одного WebSocket пользователя
type Client struct {
	conn     *websocket.Conn
	hub      *Hub
	send     chan []byte
	room     string
	username string
}

// Hub - управляет всеми клиентами и комнатами
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	rooms      map[string]map[*Client]bool
	db         Repository
}

// NewHub создаёт новый хаб
func NewHub(db Repository) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		rooms:      make(map[string]map[*Client]bool),
		db:         db,
	}
}

// removeClient делает безопасное удаление клиента из всех структур и закрывает канал отправки
func (h *Hub) removeClient(client *Client) {
	if _, exists := h.clients[client]; !exists {
		return
	}

	delete(h.clients, client)

	if clientsInRoom, ok := h.rooms[client.room]; ok {
		delete(clientsInRoom, client)
		if len(clientsInRoom) == 0 {
			delete(h.rooms, client.room)
		}
	}

	close(client.send)
}

// Run запускает главный цикл обработки событий хаба
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			if h.rooms[client.room] == nil {
				h.rooms[client.room] = make(map[*Client]bool)
			}
			h.rooms[client.room][client] = true

		case client := <-h.unregister:
			h.removeClient(client)

		case message := <-h.broadcast:
			var msg Message
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Некорректный формат сообщения: %v", err)
				continue
			}

			if h.db != nil {
				if err := h.db.SaveMessage(msg); err != nil {
					log.Printf("Не удалось сохранить сообщение: %v", err)
				}
			}

			clientsInRoom := h.rooms[msg.Room]
			if clientsInRoom == nil {
				continue
			}

			for client := range clientsInRoom {
				select {
				case client.send <- message:
					// ok
				default:
					// Если буфер отправки полон, отключаем клиента и удаляем из комнат/хаба
					h.removeClient(client)
				}
			}
		}
	}
}

// readPump читает сообщения от клиента.
func (c *Client) readPump() {
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			c.hub.unregister <- c
			return
		}

		msg := Message{
			Room:      c.room,
			Username:  c.username,
			Text:      string(message),
			Timestamp: time.Now().Unix(),
		}

		jsonMsg, err := json.Marshal(msg)
		if err != nil {
			log.Printf("Ошибка сериализации сообщения: %v", err)
			continue
		}
		c.hub.broadcast <- jsonMsg
	}
}

// writePump отправляет сообщения клиенту.
func (c *Client) writePump() {
	defer c.conn.Close()
	for {
		message, ok := <-c.send
		if !ok {
			// Канал `send` закрыт.
			_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			c.hub.unregister <- c
			return
		}
	}
}

// ServeWS обрабатывает http запрос и обновляет его до WebSocket.
func ServeWS(hub *Hub, w http.ResponseWriter, r *http.Request) {
	room := r.URL.Query().Get("room")
	username := r.URL.Query().Get("username")
	if room == "" || username == "" {
		http.Error(w, "Room and username are required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Ошибка апгрейда до WebSocket: %v", err)
		return
	}

	client := &Client{
		conn:     conn,
		hub:      hub,
		send:     make(chan []byte, 256),
		room:     room,
		username: username,
	}
	client.hub.register <- client

	go client.writePump()
	go client.readPump()
}

