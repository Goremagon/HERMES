<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const authStore = useAuthStore()

const mode = ref('login')
const username = ref('')
const password = ref('')
const formError = ref('')
const submitting = ref(false)

function toggleMode() {
  mode.value = mode.value === 'login' ? 'register' : 'login'
  formError.value = ''
}

async function submitForm() {
  formError.value = ''
  submitting.value = true

  try {
    if (mode.value === 'register') {
      await authStore.register(username.value.trim(), password.value)
    }

    await authStore.login(username.value.trim(), password.value)
    await router.push('/')
  } catch (error) {
    formError.value = error.message || 'Authentication failed'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="auth-layout">
    <section class="auth-card">
      <h1>{{ mode === 'login' ? 'Login' : 'Register' }}</h1>
      <p class="hint">OpenVoice Identity Layer</p>

      <form @submit.prevent="submitForm">
        <label for="username">Username</label>
        <input id="username" v-model="username" autocomplete="username" required />

        <label for="password">Password</label>
        <input id="password" v-model="password" type="password" autocomplete="current-password" required />

        <button type="submit" :disabled="submitting">
          {{ submitting ? 'Please wait…' : mode === 'login' ? 'Login' : 'Register & Login' }}
        </button>
      </form>

      <p v-if="formError" class="error">{{ formError }}</p>

      <button class="toggle" type="button" @click="toggleMode">
        {{ mode === 'login' ? 'Need an account? Register' : 'Already have an account? Login' }}
      </button>
    </section>
  </main>
</template>

<style scoped>
.auth-layout {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #101828;
}

.auth-card {
  width: 360px;
  padding: 2rem;
  border-radius: 12px;
  background: #1d2939;
  color: #f8fafc;
}

.hint {
  color: #98a2b3;
  margin-bottom: 1rem;
}

form {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

input {
  border: 1px solid #344054;
  border-radius: 8px;
  padding: 0.65rem 0.75rem;
  background: #0c111d;
  color: #f8fafc;
}

button {
  border: 0;
  border-radius: 8px;
  padding: 0.7rem 0.8rem;
  font-weight: 600;
  cursor: pointer;
}

button[type='submit'] {
  margin-top: 0.5rem;
  background: #7c3aed;
  color: white;
}

button:disabled {
  opacity: 0.65;
}

.error {
  margin-top: 1rem;
  color: #fda29b;
}

.toggle {
  margin-top: 0.75rem;
  background: transparent;
  color: #d0d5dd;
}
</style>
