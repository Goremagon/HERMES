<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const authStore = useAuthStore()
const router = useRouter()

const channels = ref([])
const channelName = ref('')
const loading = ref(false)
const pageError = ref('')

async function loadChannels() {
  loading.value = true
  pageError.value = ''
  try {
    const response = await fetch('/api/channels', {
      credentials: 'include',
    })

    if (response.status === 401) {
      await authStore.logout()
      await router.push('/login')
      return
    }

    const payload = await response.json()
    if (!response.ok) {
      throw new Error(payload.error || 'Failed to load channels')
    }

    channels.value = payload.channels
  } catch (error) {
    pageError.value = error.message || 'Failed to load channels'
  } finally {
    loading.value = false
  }
}

async function createChannel() {
  pageError.value = ''
  try {
    const response = await fetch('/api/channels', {
      method: 'POST',
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        name: channelName.value.trim(),
        type: 'text',
      }),
    })

    const payload = await response.json()
    if (!response.ok) {
      throw new Error(payload.error || 'Failed to create channel')
    }

    channels.value.push(payload.channel)
    channelName.value = ''
  } catch (error) {
    pageError.value = error.message || 'Failed to create channel'
  }
}

async function logout() {
  await authStore.logout()
  await router.push('/login')
}

onMounted(loadChannels)
</script>

<template>
  <main class="dashboard-layout">
    <aside class="sidebar">
      <div class="sidebar-header">
        <h2>OpenVoice</h2>
        <p v-if="authStore.user">@{{ authStore.user.username }}</p>
      </div>

      <section>
        <h3>Channels</h3>
        <ul>
          <li v-for="channel in channels" :key="channel.id"># {{ channel.name }}</li>
          <li v-if="!loading && channels.length === 0">No channels yet.</li>
        </ul>
      </section>

      <form class="channel-create" @submit.prevent="createChannel">
        <label for="channel-name">Create Channel</label>
        <input id="channel-name" v-model="channelName" maxlength="30" required placeholder="General" />
        <button type="submit">Create</button>
      </form>

      <button class="logout" @click="logout">Logout</button>
      <p v-if="pageError" class="error">{{ pageError }}</p>
    </aside>

    <section class="content">
      <h1>Dashboard</h1>
      <p>Select a channel from the sidebar to continue.</p>
    </section>
  </main>
</template>

<style scoped>
.dashboard-layout {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 280px 1fr;
}

.sidebar {
  background: #111827;
  color: #f9fafb;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.sidebar-header p {
  color: #9ca3af;
}

ul {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.channel-create {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

input {
  border-radius: 6px;
  border: 1px solid #374151;
  background: #1f2937;
  color: #f9fafb;
  padding: 0.55rem 0.65rem;
}

button {
  border: 0;
  border-radius: 6px;
  padding: 0.55rem 0.65rem;
  font-weight: 600;
  cursor: pointer;
}

.logout {
  margin-top: auto;
}

.content {
  background: #f3f4f6;
  padding: 2rem;
}

.error {
  color: #fecaca;
}
</style>
