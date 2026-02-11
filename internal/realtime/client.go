package realtime

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxPayloadSize = 8 * 1024
)

type Client struct {
	hub            *Hub
	conn           *websocket.Conn
	send           chan []byte
	user           User
	channelID      int64
	voiceChannelID int64
}

func newClient(hub *Hub, conn *websocket.Conn, user User) *Client {
	return &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
		user: user,
	}
}

func (c *Client) readPump() {
	defer func() {
		if err := c.hub.markVoiceLeave(c, c.voiceChannelID); err != nil {
			c.hub.sendError(c, "failed to leave voice")
		}
		c.hub.removeClient(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxPayloadSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var evt inboundEvent
		if err := json.Unmarshal(message, &evt); err != nil {
			c.hub.sendError(c, "invalid event payload")
			continue
		}

		switch evt.Type {
		case "join_channel":
			if err := c.hub.joinChannel(c, evt.ChannelID); err != nil {
				c.hub.sendError(c, err.Error())
				continue
			}

			history, err := c.hub.loadHistory(evt.ChannelID)
			if err != nil {
				c.hub.sendError(c, "failed to load channel history")
				continue
			}

			payload, err := json.Marshal(outboundEvent{Type: "channel_history", Data: channelHistoryData{ChannelID: evt.ChannelID, Messages: history}})
			if err != nil {
				c.hub.sendError(c, "failed to encode history")
				continue
			}
			c.send <- payload
		case "send_message":
			channelID := evt.ChannelID
			if channelID == 0 {
				channelID = c.channelID
			}

			if err := c.hub.createAndBroadcastMessage(c, channelID, evt.Content); err != nil {
				c.hub.sendError(c, err.Error())
				continue
			}
		case "join_voice":
			if err := c.hub.markVoiceJoin(c, evt.ChannelID); err != nil {
				c.hub.sendError(c, err.Error())
			}
		case "leave_voice":
			if err := c.hub.markVoiceLeave(c, evt.ChannelID); err != nil {
				c.hub.sendError(c, err.Error())
			}
		case "signal":
			if err := c.hub.relaySignal(c, evt); err != nil {
				c.hub.sendError(c, err.Error())
			}
		default:
			c.hub.sendError(c, "unsupported event type")
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
