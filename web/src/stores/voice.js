import { defineStore } from 'pinia'
import { useAuthStore } from './auth'
import { useChatStore } from './chat'

const rtcConfig = {
  iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
}

export const useVoiceStore = defineStore('voice', {
  state: () => ({
    localStream: null,
    peers: {},
    remoteStreams: {},
    participants: {},
    joinedChannelId: null,
    muted: false,
    deafened: false,
    cameraOff: false,
    error: '',
    beforeUnloadBound: false,
  }),
  getters: {
    remoteMediaEntries: (state) =>
      Object.entries(state.remoteStreams).map(([userId, stream]) => ({
        userId,
        stream,
        username: state.participants[userId] || `User ${userId}`,
      })),
    participantList: (state) => Object.entries(state.participants).map(([id, username]) => ({ id, username })),
    isConnected: (state) => Boolean(state.joinedChannelId),
  },
  actions: {
    async initialize() {
      if (this.beforeUnloadBound) {
        return
      }
      window.addEventListener('beforeunload', this.handleBeforeUnload)
      this.beforeUnloadBound = true
    },
    async joinRoom(channelId) {
      const chatStore = useChatStore()
      const authStore = useAuthStore()

      await this.initialize()

      if (!authStore.user) {
        this.error = 'User session required for voice'
        return
      }

      this.error = ''

      if (!this.localStream) {
        try {
          this.localStream = await navigator.mediaDevices.getUserMedia({ audio: true, video: true })
        } catch (error) {
          this.error = error.message || 'Microphone/Camera access denied'
          return
        }
      }

      if (this.joinedChannelId && this.joinedChannelId !== channelId) {
        this.leaveRoom()
      }

      this.joinedChannelId = channelId
      this.participants[String(authStore.user.id)] = authStore.user.username

      chatStore.connect()
      chatStore.sendEvent({ type: 'join_voice', channel_id: channelId })
    },
    leaveRoom() {
      const chatStore = useChatStore()
      const authStore = useAuthStore()

      if (this.joinedChannelId) {
        chatStore.sendEvent({ type: 'leave_voice', channel_id: this.joinedChannelId })
      }

      Object.keys(this.peers).forEach((userId) => {
        this.closePeer(userId)
      })

      if (this.localStream) {
        this.localStream.getTracks().forEach((track) => track.stop())
      }

      this.localStream = null
      this.peers = {}
      this.remoteStreams = {}
      this.participants = {}
      this.joinedChannelId = null
      this.muted = false
      this.deafened = false
      this.cameraOff = false

      if (authStore.user) {
        this.participants[String(authStore.user.id)] = authStore.user.username
      }
    },
    teardown() {
      this.leaveRoom()
      if (this.beforeUnloadBound) {
        window.removeEventListener('beforeunload', this.handleBeforeUnload)
        this.beforeUnloadBound = false
      }
    },
    toggleMute() {
      if (!this.localStream) {
        return
      }

      this.muted = !this.muted
      this.localStream.getAudioTracks().forEach((track) => {
        track.enabled = !this.muted
      })
    },
    toggleCamera() {
      if (!this.localStream) {
        return
      }

      this.cameraOff = !this.cameraOff
      this.localStream.getVideoTracks().forEach((track) => {
        track.enabled = !this.cameraOff
      })
    },
    toggleDeafen() {
      this.deafened = !this.deafened
    },
    async handleRealtimeEvent(payload) {
      const authStore = useAuthStore()
      if (!authStore.user) {
        return
      }

      const myID = String(authStore.user.id)

      if (payload.type === 'user_joined_voice') {
        const data = payload.data || {}
        if (data.channel_id !== this.joinedChannelId) {
          return
        }

        const remoteID = String(data.user_id)
        this.participants[remoteID] = data.username || `User ${remoteID}`

        if (remoteID !== myID) {
          await this.createOffer(remoteID)
        }
        return
      }

      if (payload.type === 'leave_voice') {
        const data = payload.data || {}
        if (data.channel_id !== this.joinedChannelId) {
          return
        }

        const remoteID = String(data.user_id)
        if (remoteID !== myID) {
          this.closePeer(remoteID)
          delete this.participants[remoteID]
        }
        return
      }

      if (payload.type === 'signal') {
        const data = payload.data || {}
        if (String(data.target_id || '') !== myID) {
          return
        }
        if (Number(data.channel_id) !== this.joinedChannelId) {
          return
        }

        const fromUserID = String(data.from_user_id)
        this.participants[fromUserID] = data.from_name || `User ${fromUserID}`
        await this.handleSignal(fromUserID, data.payload)
      }
    },
    async handleSignal(fromUserID, payload) {
      const signalType = payload?.type
      if (!signalType) {
        return
      }

      const pc = await this.ensurePeer(fromUserID)

      if (signalType === 'offer') {
        await pc.setRemoteDescription(
          new RTCSessionDescription({
            type: 'offer',
            sdp: payload.sdp,
          }),
        )
        const answer = await pc.createAnswer()
        await pc.setLocalDescription(answer)
        this.sendSignal(fromUserID, {
          type: 'answer',
          sdp: answer.sdp,
        })
        return
      }

      if (signalType === 'answer') {
        await pc.setRemoteDescription(
          new RTCSessionDescription({
            type: 'answer',
            sdp: payload.sdp,
          }),
        )
        return
      }

      if (signalType === 'ice-candidate' && payload.candidate) {
        await pc.addIceCandidate(payload.candidate)
      }
    },
    async createOffer(targetUserID) {
      const pc = await this.ensurePeer(targetUserID)
      const offer = await pc.createOffer()
      await pc.setLocalDescription(offer)
      this.sendSignal(targetUserID, {
        type: 'offer',
        sdp: offer.sdp,
      })
    },
    async ensurePeer(remoteUserID) {
      if (!this.localStream) {
        throw new Error('Local stream is not initialized')
      }

      if (this.peers[remoteUserID]) {
        return this.peers[remoteUserID]
      }

      const pc = new RTCPeerConnection(rtcConfig)

      this.localStream.getTracks().forEach((track) => {
        pc.addTrack(track, this.localStream)
      })

      pc.onicecandidate = (event) => {
        if (event.candidate) {
          this.sendSignal(remoteUserID, {
            type: 'ice-candidate',
            candidate: event.candidate,
          })
        }
      }

      pc.ontrack = (event) => {
        const [stream] = event.streams
        if (stream) {
          this.remoteStreams[remoteUserID] = stream
        }
      }

      pc.onconnectionstatechange = () => {
        if (['failed', 'closed', 'disconnected'].includes(pc.connectionState)) {
          this.closePeer(remoteUserID)
        }
      }

      this.peers[remoteUserID] = pc
      return pc
    },
    sendSignal(targetUserID, payload) {
      const chatStore = useChatStore()
      if (!this.joinedChannelId) {
        return
      }

      chatStore.sendEvent({
        type: 'signal',
        target_id: String(targetUserID),
        channel_id: this.joinedChannelId,
        payload,
      })
    },
    closePeer(remoteUserID) {
      const pc = this.peers[remoteUserID]
      if (pc) {
        pc.ontrack = null
        pc.onicecandidate = null
        pc.onconnectionstatechange = null
        pc.close()
      }
      delete this.peers[remoteUserID]
      delete this.remoteStreams[remoteUserID]
    },
    handleBeforeUnload: function () {
      this.leaveRoom()
    },
  },
})
