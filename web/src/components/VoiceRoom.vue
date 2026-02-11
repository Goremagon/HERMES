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
const audioRefs = ref({})

function setAudioRef(el, userId) {
  if (!el) {
    delete audioRefs.value[userId]
    return
  }
  audioRefs.value[userId] = el
  const stream = voiceStore.remoteStreams[userId]
  if (stream) {
    el.srcObject = stream
    el.muted = voiceStore.deafened
  }
}

watch(
  () => voiceStore.remoteAudioEntries,
  async () => {
    await nextTick()
    voiceStore.remoteAudioEntries.forEach((entry) => {
      const el = audioRefs.value[entry.userId]
      if (el) {
        el.srcObject = entry.stream
        el.muted = voiceStore.deafened
      }
    })
  },
  { deep: true },
)

watch(
  () => voiceStore.deafened,
  () => {
    Object.values(audioRefs.value).forEach((el) => {
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
  <section class="voice-room">
    <header>
      <h3>Voice</h3>
      <p v-if="voiceStore.joinedChannelId">Connected</p>
      <p v-else>Disconnected</p>
    </header>

    <div class="controls">
      <button v-if="!voiceStore.joinedChannelId" :disabled="!channelId" @click="joinVoice">Join Voice</button>
      <template v-else>
        <button @click="voiceStore.toggleMute">{{ voiceStore.muted ? 'Unmute' : 'Mute' }}</button>
        <button @click="voiceStore.toggleDeafen">{{ voiceStore.deafened ? 'Undeafen' : 'Deafen' }}</button>
        <button @click="leaveVoice">Leave</button>
      </template>
    </div>

    <ul>
      <li v-for="participant in voiceStore.participantList" :key="participant.id">{{ participant.username }}</li>
    </ul>

    <audio
      v-for="entry in voiceStore.remoteAudioEntries"
      :key="entry.userId"
      autoplay
      :ref="(el) => setAudioRef(el, entry.userId)"
    />

    <p v-if="voiceStore.error" class="error">{{ voiceStore.error }}</p>
  </section>
</template>

<style scoped>
.voice-room {
  border-top: 1px solid #374151;
  margin-top: 0.75rem;
  padding-top: 0.75rem;
}

.controls {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin-bottom: 0.5rem;
}

ul {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.error {
  color: #fecaca;
  margin-top: 0.5rem;
}
</style>
