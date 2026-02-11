package realtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"openvoice/internal/database"

	"github.com/gorilla/websocket"
)

const (
	maxMessageSize   = 16 * 1024
	messageHistLimit = 50
)

type Hub struct {
	db       *sql.DB
	mu       sync.Mutex
	clients  map[*Client]struct{}
	channels map[int64]map[*Client]struct{}
	upgrader websocket.Upgrader
}

type User struct {
	ID       int64
	Username string
}

type inboundEvent struct {
	Type      string          `json:"type"`
	ChannelID int64           `json:"channel_id"`
	Content   string          `json:"content"`
	TargetID  string          `json:"target_id"`
	Payload   json.RawMessage `json:"payload"`
}

type outboundEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type channelHistoryData struct {
	ChannelID int64              `json:"channel_id"`
	Messages  []database.Message `json:"messages"`
}

type signalData struct {
	FromUserID int64           `json:"from_user_id"`
	FromName   string          `json:"from_name"`
	TargetID   string          `json:"target_id"`
	ChannelID  int64           `json:"channel_id"`
	Payload    json.RawMessage `json:"payload"`
}

type voicePresenceData struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	ChannelID int64  `json:"channel_id"`
}

func NewHub(db *sql.DB) *Hub {
	return &Hub{
		db:       db,
		clients:  make(map[*Client]struct{}),
		channels: make(map[int64]map[*Client]struct{}),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				host := r.Host
				return strings.Contains(origin, host)
			},
		},
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, user User) error {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("upgrade websocket: %w", err)
	}

	client := newClient(h, conn, user)
	h.addClient(client)

	go client.writePump()
	go client.readPump()

	return nil
}

func (h *Hub) addClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client] = struct{}{}
}

func (h *Hub) removeClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.clients, client)
	for channelID, members := range h.channels {
		delete(members, client)
		if len(members) == 0 {
			delete(h.channels, channelID)
		}
	}
}

func (h *Hub) joinChannel(client *Client, channelID int64) error {
	if channelID <= 0 {
		return fmt.Errorf("invalid channel id")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var exists int
	if err := h.db.QueryRowContext(ctx, `SELECT 1 FROM channels WHERE id = ?`, channelID).Scan(&exists); err != nil {
		return fmt.Errorf("channel not found")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for cid, members := range h.channels {
		if _, ok := members[client]; ok {
			delete(members, client)
			if len(members) == 0 {
				delete(h.channels, cid)
			}
		}
	}

	if _, ok := h.channels[channelID]; !ok {
		h.channels[channelID] = make(map[*Client]struct{})
	}
	h.channels[channelID][client] = struct{}{}
	client.channelID = channelID

	return nil
}

func (h *Hub) markVoiceJoin(client *Client, channelID int64) error {
	if err := h.joinChannel(client, channelID); err != nil {
		return err
	}

	client.voiceChannelID = channelID
	presence := voicePresenceData{UserID: client.user.ID, Username: client.user.Username, ChannelID: channelID}
	encoded, err := json.Marshal(outboundEvent{Type: "user_joined_voice", Data: presence})
	if err != nil {
		return fmt.Errorf("marshal user_joined_voice: %w", err)
	}
	h.broadcastToChannel(channelID, encoded)
	log.Printf("user %d joined voice channel %d", client.user.ID, channelID)
	return nil
}

func (h *Hub) markVoiceLeave(client *Client, channelID int64) error {
	if channelID <= 0 {
		channelID = client.voiceChannelID
	}
	if channelID <= 0 {
		return nil
	}

	client.voiceChannelID = 0
	presence := voicePresenceData{UserID: client.user.ID, Username: client.user.Username, ChannelID: channelID}
	encoded, err := json.Marshal(outboundEvent{Type: "leave_voice", Data: presence})
	if err != nil {
		return fmt.Errorf("marshal leave_voice: %w", err)
	}
	h.broadcastToChannel(channelID, encoded)
	log.Printf("user %d left voice channel %d", client.user.ID, channelID)
	return nil
}

func (h *Hub) relaySignal(client *Client, evt inboundEvent) error {
	channelID := evt.ChannelID
	if channelID <= 0 {
		channelID = client.voiceChannelID
	}
	if channelID <= 0 {
		channelID = client.channelID
	}
	if channelID <= 0 {
		return fmt.Errorf("channel is required for signal")
	}

	if len(evt.Payload) == 0 {
		return fmt.Errorf("signal payload is required")
	}

	msg := outboundEvent{Type: "signal", Data: signalData{
		FromUserID: client.user.ID,
		FromName:   client.user.Username,
		TargetID:   evt.TargetID,
		ChannelID:  channelID,
		Payload:    evt.Payload,
	}}
	encoded, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal signal: %w", err)
	}

	h.broadcastToChannel(channelID, encoded)
	log.Printf("signal relay from user %d to target %s on channel %d", client.user.ID, evt.TargetID, channelID)
	return nil
}

func (h *Hub) loadHistory(channelID int64) ([]database.Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	messages, err := database.GetMessages(ctx, h.db, channelID, messageHistLimit)
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	return messages, nil
}

func (h *Hub) createAndBroadcastMessage(client *Client, channelID int64, content string) error {
	if channelID <= 0 {
		return fmt.Errorf("invalid channel id")
	}

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("message content is required")
	}
	if len(trimmed) > maxMessageSize {
		return fmt.Errorf("message content too long")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	message, err := database.CreateMessage(ctx, h.db, client.user.ID, channelID, trimmed)
	if err != nil {
		return fmt.Errorf("create message: %w", err)
	}

	payload := outboundEvent{Type: "new_message", Data: message}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal outbound message: %w", err)
	}

	h.broadcastToChannel(channelID, encoded)
	return nil
}

func (h *Hub) broadcastToChannel(channelID int64, data []byte) {
	h.mu.Lock()
	members := make([]*Client, 0)
	for client := range h.channels[channelID] {
		members = append(members, client)
	}
	h.mu.Unlock()

	for _, client := range members {
		select {
		case client.send <- data:
		default:
			h.removeClient(client)
			_ = client.conn.Close()
		}
	}
}

func (h *Hub) sendError(client *Client, message string) {
	payload, err := json.Marshal(outboundEvent{Type: "error", Data: map[string]string{"message": message}})
	if err != nil {
		return
	}

	select {
	case client.send <- payload:
	default:
	}
}
