import { defineStore } from 'pinia'
import { ref } from 'vue'

const STORAGE_KEY = 'theme'

export const useThemeStore = defineStore('theme', () => {
  const theme = ref('light') // 'light' | 'dark'

  function apply(value) {
    theme.value = value
    document.documentElement.classList.toggle('dark', value === 'dark')
  }

  /** Read persisted theme (or system preference), apply it to <html>. */
  function init() {
    let saved = null
    try { saved = localStorage.getItem(STORAGE_KEY) } catch { /* no-op */ }
    const value = saved === 'dark' || saved === 'light'
      ? saved
      : (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
    apply(value)
  }

  function toggle() {
    const next = theme.value === 'dark' ? 'light' : 'dark'
    apply(next)
    try { localStorage.setItem(STORAGE_KEY, next) } catch { /* no-op */ }
  }

  return { theme, init, toggle }
})
