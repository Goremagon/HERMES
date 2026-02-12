import { defineStore } from 'pinia'

const jsonHeaders = {
  'Content-Type': 'application/json',
}

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null,
    initialized: false,
    loading: false,
    error: '',
  }),
  getters: {
    isAuthenticated: (state) => Boolean(state.user),
  },
  actions: {
    async initialize() {
      if (this.initialized) {
        return
      }
      await this.fetchMe()
      this.initialized = true
    },
    async fetchMe() {
      this.loading = true
      this.error = ''
      try {
        const response = await fetch('/api/me', {
          credentials: 'include',
        })

        if (response.status === 401) {
          this.user = null
          return
        }

        if (!response.ok) {
          throw new Error('Failed to load user session')
        }

        const payload = await response.json()
        this.user = payload.user
      } catch (error) {
        this.user = null
        this.error = error.message || 'Failed to load user session'
      } finally {
        this.loading = false
      }
    },
    async register(username, password) {
      const response = await fetch('/api/register', {
        method: 'POST',
        headers: jsonHeaders,
        body: JSON.stringify({ username, password }),
      })

      const payload = await response.json()
      if (!response.ok) {
        throw new Error(payload.error || 'Registration failed')
      }

      return payload.user
    },
    async login(username, password) {
      const response = await fetch('/api/login', {
        method: 'POST',
        headers: jsonHeaders,
        credentials: 'include',
        body: JSON.stringify({ username, password }),
      })

      const payload = await response.json()
      if (!response.ok) {
        throw new Error(payload.error || 'Login failed')
      }

      this.user = payload.user
      return payload.user
    },
    async logout() {
      await fetch('/api/logout', {
        method: 'POST',
        credentials: 'include',
      })
      this.user = null
    },
  },
})
