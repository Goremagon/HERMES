import { defineStore } from 'pinia'

const wsScheme = window.location.protocol === 'https:' ? 'wss' : 'ws'

export const useChatStore = defineStore('chat', {
  state: () => ({
    ws: null,
    connected: false,
    messages: [],
    activeChannelId: null,
    error: '',
  }),
  actions: {
    connect() {
      if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
        return
      }

      this.ws = new WebSocket(`${wsScheme}://${window.location.host}/api/ws`)

      this.ws.addEventListener('open', () => {
        this.connected = true
        this.error = ''
        if (this.activeChannelId) {
          this.joinChannel(this.activeChannelId)
        }
      })

      this.ws.addEventListener('close', () => {
        this.connected = false
      })

      this.ws.addEventListener('message', (event) => {
        let payload
        try {
          payload = JSON.parse(event.data)
        } catch {
          this.error = 'Invalid realtime payload'
          return
        }

        if (payload.type === 'channel_history') {
          this.messages = payload.data?.messages || []
          return
        }

        if (payload.type === 'new_message') {
          if (payload.data?.channel_id === this.activeChannelId) {
            this.messages.push(payload.data)
          }
          return
        }

        if (payload.type === 'error') {
          this.error = payload.data?.message || 'Realtime error'
        }
      })
    },
    disconnect() {
      if (this.ws) {
        this.ws.close()
      }
      this.ws = null
      this.connected = false
      this.activeChannelId = null
      this.messages = []
    },
    joinChannel(channelId) {
      this.activeChannelId = channelId
      this.messages = []
      if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
        this.connect()
        return
      }
      this.ws.send(
        JSON.stringify({
          type: 'join_channel',
          channel_id: channelId,
        }),
      )
    },
    sendMessage(content) {
      if (!this.activeChannelId || !this.ws || this.ws.readyState !== WebSocket.OPEN) {
        this.error = 'Not connected to chat'
        return
      }

      this.ws.send(
        JSON.stringify({
          type: 'send_message',
          channel_id: this.activeChannelId,
          content,
        }),
      )
    },
  },
})
