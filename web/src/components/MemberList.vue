<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'

const members = ref([])
const loading = ref(false)
const error = ref('')
let pollTimer

async function fetchMembers() {
  loading.value = true
  error.value = ''
  try {
    const response = await fetch('/api/users', {
      credentials: 'include',
    })
    const payload = await response.json()
    if (!response.ok) {
      throw new Error(payload.error || 'Failed to load members')
    }
    members.value = payload.users || []
  } catch (err) {
    error.value = err.message || 'Failed to load members'
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await fetchMembers()
  pollTimer = setInterval(fetchMembers, 15000)
})

onBeforeUnmount(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
  }
})
</script>

<template>
  <aside class="member-list">
    <h3>Members</h3>
    <p v-if="loading && members.length === 0">Loading members...</p>
    <p v-if="error" class="error">{{ error }}</p>

    <ul>
      <li v-for="member in members" :key="member.id">
        <img v-if="member.avatar_url" :src="member.avatar_url" alt="avatar" class="avatar" />
        <div v-else class="avatar placeholder">{{ member.username.slice(0, 1).toUpperCase() }}</div>
        <span class="name">{{ member.username }}</span>
        <span class="status" :class="{ online: member.online }">{{ member.online ? 'online' : 'offline' }}</span>
      </li>
    </ul>
  </aside>
</template>

<style scoped>
.member-list {
  background: #0b1220;
  border-left: 1px solid #1f2937;
  color: #e5e7eb;
  padding: 1rem;
}

ul {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

li {
  display: grid;
  grid-template-columns: 34px 1fr auto;
  gap: 0.55rem;
  align-items: center;
}

.avatar {
  width: 34px;
  height: 34px;
  border-radius: 999px;
  object-fit: cover;
}

.placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  background: #374151;
  color: #fff;
  font-weight: 700;
}

.name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status {
  font-size: 0.75rem;
  color: #9ca3af;
}

.status.online {
  color: #34d399;
}

.error {
  color: #fca5a5;
}

@media (max-width: 900px) {
  .member-list {
    display: none;
  }
}
</style>
