<script setup>
import { onMounted, ref } from 'vue'
import { useAuthStore } from '../stores/auth'
import { useVoiceStore } from '../stores/voice'

const authStore = useAuthStore()
const voiceStore = useVoiceStore()

const tab = ref('account')
const username = ref('')
const avatarURL = ref('')
const profileMessage = ref('')
const settingsMessage = ref('')
const saving = ref(false)

function initProfile() {
  username.value = authStore.user?.username || ''
  avatarURL.value = authStore.user?.avatar_url || ''
}

async function uploadAvatar(event) {
  const file = event.target.files?.[0]
  if (!file) {
    return
  }

  const formData = new FormData()
  formData.append('file', file)

  try {
    const response = await fetch('/api/upload', {
      method: 'POST',
      credentials: 'include',
      body: formData,
    })

    const payload = await response.json()
    if (!response.ok) {
      throw new Error(payload.error || 'Upload failed')
    }

    avatarURL.value = payload.url
    profileMessage.value = 'Avatar uploaded'
  } catch (error) {
    profileMessage.value = error.message || 'Upload failed'
  } finally {
    event.target.value = ''
  }
}

async function saveProfile() {
  saving.value = true
  profileMessage.value = ''
  try {
    const response = await fetch('/api/me', {
      method: 'PUT',
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        username: username.value.trim(),
        avatar_url: avatarURL.value.trim(),
      }),
    })

    const payload = await response.json()
    if (!response.ok) {
      throw new Error(payload.error || 'Failed to update profile')
    }

    authStore.user = payload.user
    profileMessage.value = 'Profile updated'
  } catch (error) {
    profileMessage.value = error.message || 'Failed to update profile'
  } finally {
    saving.value = false
  }
}

function saveDevicePreferences() {
  voiceStore.savePreferences()
  settingsMessage.value = 'Device preferences saved'
}

onMounted(async () => {
  await authStore.fetchMe()
  await voiceStore.listDevices()
  initProfile()
})
</script>

<template>
  <main class="settings-layout">
    <aside class="tabs">
      <button :class="{ active: tab === 'account' }" @click="tab = 'account'">Account</button>
      <button :class="{ active: tab === 'devices' }" @click="tab = 'devices'">Voice & Video</button>
    </aside>

    <section v-if="tab === 'account'" class="panel">
      <h1>Profile Settings</h1>
      <label>Username</label>
      <input v-model="username" maxlength="20" />

      <label>Avatar</label>
      <input type="file" accept=".jpg,.jpeg,.png,.gif,.webm" @change="uploadAvatar" />
      <img v-if="avatarURL" :src="avatarURL" class="avatar-preview" alt="avatar preview" />

      <button :disabled="saving" @click="saveProfile">Save Profile</button>
      <p v-if="profileMessage" class="message">{{ profileMessage }}</p>
    </section>

    <section v-else class="panel">
      <h1>Voice & Video Settings</h1>

      <label>Microphone</label>
      <select v-model="voiceStore.selectedAudioInputId">
        <option value="">Default Microphone</option>
        <option v-for="device in voiceStore.audioInputs" :key="device.deviceId" :value="device.deviceId">
          {{ device.label || `Microphone ${device.deviceId.slice(0, 6)}` }}
        </option>
      </select>

      <label>Camera</label>
      <select v-model="voiceStore.selectedVideoInputId">
        <option value="">Default Camera</option>
        <option v-for="device in voiceStore.videoInputs" :key="device.deviceId" :value="device.deviceId">
          {{ device.label || `Camera ${device.deviceId.slice(0, 6)}` }}
        </option>
      </select>

      <button @click="saveDevicePreferences">Save Device Preferences</button>
      <p v-if="settingsMessage" class="message">{{ settingsMessage }}</p>
    </section>
  </main>
</template>

<style scoped>
.settings-layout {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 220px 1fr;
  color: #e5e7eb;
}

.tabs {
  background: #0b1220;
  border-right: 1px solid #1f2937;
  display: flex;
  flex-direction: column;
  padding: 1rem;
  gap: 0.5rem;
}

.tabs button {
  text-align: left;
  background: transparent;
  color: #e5e7eb;
  border: 1px solid #334155;
}

.tabs button.active {
  background: #1d4ed8;
}

.panel {
  padding: 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

input,
select {
  border: 1px solid #334155;
  border-radius: 8px;
  padding: 0.55rem;
  background: #0f172a;
  color: #e5e7eb;
}

.avatar-preview {
  width: 80px;
  height: 80px;
  object-fit: cover;
  border-radius: 999px;
  border: 2px solid #1d4ed8;
}

button {
  width: fit-content;
  border: 0;
  border-radius: 8px;
  padding: 0.55rem 0.8rem;
  background: #2563eb;
  color: #fff;
}

.message {
  color: #bfdbfe;
}
</style>
