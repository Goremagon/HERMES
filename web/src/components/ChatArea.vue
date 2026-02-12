<script setup>
import { computed, nextTick, ref, watch } from 'vue'

const props = defineProps({
  channelName: {
    type: String,
    default: '',
  },
  messages: {
    type: Array,
    default: () => [],
  },
  disabled: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['send'])

const draft = ref('')
const listRef = ref(null)
const fileInputRef = ref(null)
const uploadError = ref('')

const canSend = computed(() => !props.disabled && draft.value.trim().length > 0)

function submitMessage() {
  if (!canSend.value) {
    return
  }

  emit('send', draft.value.trim())
  draft.value = ''
}

async function scrollToBottom() {
  await nextTick()
  if (listRef.value) {
    listRef.value.scrollTop = listRef.value.scrollHeight
  }
}

watch(
  () => props.messages.length,
  () => {
    scrollToBottom()
  },
  { immediate: true },
)

function openFilePicker() {
  uploadError.value = ''
  fileInputRef.value?.click()
}

async function uploadFile(event) {
  uploadError.value = ''
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

    draft.value = `${draft.value}${draft.value ? ' ' : ''}![img](${payload.url})`
  } catch (error) {
    uploadError.value = error.message || 'Upload failed'
  } finally {
    event.target.value = ''
  }
}

function parseMessageContent(content) {
  const imagePattern = /!\[[^\]]*\]\(([^)]+)\)/g
  const parts = []
  let lastIndex = 0
  let match

  while ((match = imagePattern.exec(content)) !== null) {
    if (match.index > lastIndex) {
      parts.push({ type: 'text', value: content.slice(lastIndex, match.index) })
    }
    parts.push({ type: 'image', value: match[1] })
    lastIndex = imagePattern.lastIndex
  }

  if (lastIndex < content.length) {
    parts.push({ type: 'text', value: content.slice(lastIndex) })
  }

  return parts.length > 0 ? parts : [{ type: 'text', value: content }]
}
</script>

<template>
  <section class="chat-area">
    <header>
      <h2 v-if="channelName"># {{ channelName }}</h2>
      <h2 v-else>Select a channel</h2>
    </header>

    <div ref="listRef" class="message-list">
      <p v-if="messages.length === 0" class="empty">No messages yet.</p>
      <article v-for="message in messages" :key="message.id" class="message-item">
        <span class="author">{{ message.username }}</span>
        <template v-for="(part, index) in parseMessageContent(message.content)" :key="`${message.id}-${index}`">
          <span v-if="part.type === 'text'" class="content">{{ part.value }}</span>
          <img v-else class="attachment" :src="part.value" alt="attachment" />
        </template>
      </article>
    </div>

    <form class="composer" @submit.prevent="submitMessage">
      <button type="button" class="clip" :disabled="disabled" @click="openFilePicker">📎</button>
      <input ref="fileInputRef" type="file" class="hidden" accept=".jpg,.jpeg,.png,.gif,.webm" @change="uploadFile" />
      <input
        v-model="draft"
        :disabled="disabled"
        type="text"
        placeholder="Type a message..."
        maxlength="2048"
      />
      <button type="submit" :disabled="!canSend">Send</button>
    </form>
    <p v-if="uploadError" class="error">{{ uploadError }}</p>
  </section>
</template>

<style scoped>
.chat-area {
  display: flex;
  flex-direction: column;
  height: 100vh;
}

header {
  border-bottom: 1px solid #e5e7eb;
  padding: 1rem;
}

.message-list {
  flex: 1;
  overflow-y: auto;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.message-item {
  background: #ffffff;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  padding: 0.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.author {
  font-weight: 700;
  margin-right: 0.5rem;
}

.empty {
  color: #6b7280;
}

.composer {
  border-top: 1px solid #e5e7eb;
  padding: 0.75rem;
  display: flex;
  gap: 0.5rem;
}

.hidden {
  display: none;
}

.clip {
  padding: 0.6rem 0.8rem;
}

input {
  flex: 1;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: 0.6rem;
}

button {
  border: 0;
  border-radius: 8px;
  padding: 0.6rem 1rem;
  background: #4f46e5;
  color: #fff;
  font-weight: 600;
}

button:disabled {
  opacity: 0.5;
}

.attachment {
  max-width: 280px;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
}

.error {
  color: #dc2626;
  padding: 0 0.75rem 0.5rem;
}
</style>
