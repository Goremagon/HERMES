<script setup>
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useVoiceStore } from '../stores/voice'

const props = defineProps({
  channelId: {
    type: Number,
    default: 0,
  },
})

const voiceStore = useVoiceStore()
const mediaRefs = ref({})
const localVideoRef = ref(null)

function setMediaRef(el, userId) {
  if (!el) {
    delete mediaRefs.value[userId]
    return
  }

  mediaRefs.value[userId] = el
  const stream = voiceStore.remoteStreams[userId]
  if (stream) {
    el.srcObject = stream
    el.volume = voiceStore.getPeerVolume(userId)
    el.muted = voiceStore.deafened
  }
}

function setPeerVolume(userId, value) {
  const volume = Number(value)
  voiceStore.setPeerVolume(userId, volume)
  const el = mediaRefs.value[userId]
  if (el) {
    el.volume = volume
  }
}

watch(
  () => voiceStore.localStream,
  async () => {
    await nextTick()
    if (localVideoRef.value) {
      localVideoRef.value.srcObject = voiceStore.localStream
      localVideoRef.value.muted = true
    }
  },
)

watch(
  () => voiceStore.remoteMediaEntries,
  async () => {
    await nextTick()
    voiceStore.remoteMediaEntries.forEach((entry) => {
      const el = mediaRefs.value[entry.userId]
      if (el) {
        el.srcObject = entry.stream
        el.muted = voiceStore.deafened
        el.volume = voiceStore.getPeerVolume(entry.userId)
      }
    })
  },
  { deep: true },
)

watch(
  () => voiceStore.deafened,
  () => {
    Object.values(mediaRefs.value).forEach((el) => {
      el.muted = voiceStore.deafened
    })
  },
)

function joinVoice() {
  if (!props.channelId) {
    return
  }
  voiceStore.joinRoom(props.channelId)
}

function leaveVoice() {
  voiceStore.leaveRoom()
}

onBeforeUnmount(() => {
  voiceStore.teardown()
})
</script>

<template>
  <section class="voice-room" :class="{ connected: voiceStore.isConnected }">
    <header>
      <h3>Voice</h3>
      <p v-if="voiceStore.isConnected" class="state connected-text">Connected</p>
      <p v-else class="state">Disconnected</p>
    </header>

    <div class="controls">
      <button v-if="!voiceStore.isConnected" :disabled="!channelId" @click="joinVoice">Join Voice</button>
      <template v-else>
        <button @click="voiceStore.toggleMute">{{ voiceStore.muted ? 'Unmute' : 'Mute' }}</button>
        <button @click="voiceStore.toggleCamera">{{ voiceStore.cameraOff ? 'Camera On' : 'Camera Off' }}</button>
        <button @click="voiceStore.toggleDeafen">{{ voiceStore.deafened ? 'Undeafen' : 'Deafen' }}</button>
        <button @click="leaveVoice">Leave</button>
      </template>
    </div>

    <p class="label">Connected Users</p>
    <ul>
      <li v-for="participant in voiceStore.participantList" :key="participant.id">{{ participant.username }}</li>
    </ul>

    <div class="video-grid">
      <div v-if="voiceStore.localStream" class="tile">
        <video ref="localVideoRef" autoplay playsinline muted class="video-card local" />
        <p class="caption">You</p>
      </div>

      <div v-for="entry in voiceStore.remoteMediaEntries" :key="entry.userId" class="tile">
        <video
          autoplay
          playsinline
          :ref="(el) => setMediaRef(el, entry.userId)"
          class="video-card"
        />
        <p class="caption">{{ entry.username }}</p>
        <label class="volume">
          Volume
          <input
            type="range"
            min="0"
            max="1"
            step="0.05"
            :value="voiceStore.getPeerVolume(entry.userId)"
            @input="(e) => setPeerVolume(entry.userId, e.target.value)"
          />
        </label>
      </div>
    </div>

    <p v-if="voiceStore.error" class="error">{{ voiceStore.error }}</p>
  </section>
</template>

<style scoped>
.voice-room {
  border-top: 1px solid #374151;
  border-left: 3px solid transparent;
  margin-top: 0.75rem;
  padding-top: 0.75rem;
  padding-left: 0.5rem;
}

.voice-room.connected {
  border-left-color: #22c55e;
}

.state {
  color: #d1d5db;
}

.connected-text {
  color: #86efac;
}

.controls {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin-bottom: 0.5rem;
}

.label {
  color: #d1d5db;
  font-size: 0.85rem;
  margin: 0.35rem 0;
}

ul {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.video-grid {
  margin-top: 0.6rem;
  display: grid;
  grid-template-columns: 1fr;
  gap: 0.6rem;
}

.tile {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
}

.video-card {
  width: 100%;
  border-radius: 8px;
  background: #000;
  min-height: 80px;
}

.video-card.local {
  border: 2px solid #22c55e;
}

.caption {
  margin: 0;
  font-size: 0.8rem;
  color: #cbd5e1;
}

.volume {
  font-size: 0.8rem;
  color: #cbd5e1;
}

.error {
  color: #fecaca;
  margin-top: 0.5rem;
}
</style>
