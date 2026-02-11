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

const canSend = computed(() => !props.disabled && draft.value.trim().length > 0)

function submitMessage() {
  if (!canSend.value) {
    return
  }

  emit('send', draft.value.trim())
  draft.value = ''
}

watch(
  () => props.messages.length,
  async () => {
    await nextTick()
    if (listRef.value) {
      listRef.value.scrollTop = listRef.value.scrollHeight
    }
  },
)
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
        <span class="content">{{ message.content }}</span>
      </article>
    </div>

    <form class="composer" @submit.prevent="submitMessage">
      <input
        v-model="draft"
        :disabled="disabled"
        type="text"
        placeholder="Type a message..."
        maxlength="2048"
      />
      <button type="submit" :disabled="!canSend">Send</button>
    </form>
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
</style>
