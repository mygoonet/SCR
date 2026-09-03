<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useThemeStore } from '../stores/theme'

const themeStore = useThemeStore()
const notes = ref([])
const status = ref(null)
const loading = ref(true)
const error = ref('')
const query = ref('')
const isUpdating = ref(false)
const countdown = ref(5)
const countdownPulse = ref(false)
const now = ref(Date.now())
let timer = null
let flashTimer = null
let countdownTimer = null
let clockTimer = null

async function fetchData() {
  isUpdating.value = true
  try {
    const [r1, r2] = await Promise.all([fetch('/api/notes'), fetch('/api/status')])
    if (!r1.ok) throw new Error('notes ' + r1.status)
    notes.value = await r1.json()
    // TODO: убрать после тестов — искусственная ошибка для проверки UI
    notes.value.push({
      number: 'ТСТ-000001',
      date: '31.08.2026',
      consignor: 'Тестовый отправитель',
      consignee: 'Тестовый получатель',
      carrier: 'Тестовый перевозчик',
      status: 'failed',
      error: 'Ошибка подписания: сертификат не найден',
      createdAt: '2026-08-31 10:00:00',
    })
    status.value = await r2.json()
    error.value = ''
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
    clearTimeout(flashTimer)
    flashTimer = setTimeout(() => (isUpdating.value = false), 900)
  }
}

onMounted(() => {
  fetchData()
  timer = setInterval(fetchData, 5000)
  countdown.value = 5
  countdownTimer = setInterval(() => {
    countdown.value--
    if (countdown.value <= 0) {
      countdown.value = 5
      countdownPulse.value = true
      setTimeout(() => (countdownPulse.value = false), 600)
    }
  }, 1000)
  clockTimer = setInterval(() => { now.value = Date.now() }, 1000)
})
onUnmounted(() => {
  timer && clearInterval(timer)
  countdownTimer && clearInterval(countdownTimer)
  clockTimer && clearInterval(clockTimer)
})

function signedAt(n) {
  if (n.signedAt?.length) return n.signedAt.join(', ')
  return n.processedAt || '—'
}

// Поисковые подписи статусов: сырое значение + то, что видно в таблице (Sign/ошибка) + естественные русские синонимы
const STATUS_LABELS = {
  signed: 'sign подписана',
  failed: 'ошибка error',
  in_progress: 'в работе прогресс'
}

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return notes.value
  return notes.value.filter(n =>
    n.number.toLowerCase().includes(q) ||
    (n.consignor && n.consignor.toLowerCase().includes(q)) ||
    (n.consignee && n.consignee.toLowerCase().includes(q)) ||
    (n.deliveryAddress && n.deliveryAddress.toLowerCase().includes(q)) ||
    (n.driver && n.driver.toLowerCase().includes(q)) ||
    (n.truck && n.truck.toLowerCase().includes(q)) ||
    (n.status && (
      n.status.toLowerCase().includes(q) ||
      (STATUS_LABELS[n.status] && STATUS_LABELS[n.status].includes(q))
    ))
  )
})

// carousel
const showCarousel = ref(false)
const cShots = ref([])
const cIndex = ref(0)
const cNumber = ref('')

function openCarousel(n, idx) {
  cShots.value = n.shots || []
  cIndex.value = idx
  cNumber.value = n.number
  showCarousel.value = true
}
function closeCarousel() { showCarousel.value = false }
function prev() { cIndex.value = (cIndex.value - 1 + cShots.value.length) % cShots.value.length }
function next() { cIndex.value = (cIndex.value + 1) % cShots.value.length }
function onKey(e) {
  if (!showCarousel.value) return
  if (e.key === 'Escape') closeCarousel()
  if (e.key === 'ArrowLeft') prev()
  if (e.key === 'ArrowRight') next()
}
watch(showCarousel, v => {
  if (v) { document.body.style.overflow = 'hidden'; window.addEventListener('keydown', onKey) }
  else { document.body.style.overflow = ''; window.removeEventListener('keydown', onKey) }
})
function shotUrl(name) { return `/screenshots/${cNumber.value}/${name}` }
function splitDT(s) {
  if (!s || s === '—') return { d: s, t: '' }
  const [d, t] = s.split(' ')
  return { d: d || s, t: t || '' }
}
const tickerProgress = computed(() => {
  now.value
  const t = status.value?.lastFetchTime
  if (!t) return 0
  const [d, timePart] = t.split(' ')
  if (!timePart) return 0
  const [h, m, s] = timePart.split(':').map(Number)
  const fetchDate = new Date()
  const [dd, mm, yyyy] = (d || '').split('.')
  if (yyyy) fetchDate.setFullYear(+yyyy, +mm - 1, +dd)
  fetchDate.setHours(h || 0, m || 0, s || 0, 0)
  const secSince = Math.max(0, Math.floor((Date.now() - fetchDate.getTime()) / 1000))
  const intervalSec = 6 * 60
  return Math.min(100, Math.max(0, (secSince / intervalSec) * 100))
})

const secondsSinceFetch = computed(() => {
  now.value
  const t = status.value?.lastFetchTime
  if (!t) return null
  const [d, timePart] = t.split(' ')
  if (!timePart) return null
  const [h, m, s] = timePart.split(':').map(Number)
  const fetchDate = new Date()
  const [dd, mm, yyyy] = (d || '').split('.')
  if (yyyy) fetchDate.setFullYear(+yyyy, +mm - 1, +dd)
  fetchDate.setHours(h || 0, m || 0, s || 0, 0)
  return Math.max(0, Math.floor((Date.now() - fetchDate.getTime()) / 1000))
})

const liveNow = computed(() => {
  const d = new Date(now.value)
  const pad = n => String(n).padStart(2, '0')
  return `${pad(d.getDate())}.${pad(d.getMonth()+1)}.${d.getFullYear()} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
})

function syncNow() {
  fetchData()
  countdown.value = 5
}

function formatSec(sec) {
  if (sec == null) return '—'
  const mm = Math.floor(sec / 60)
  const ss = sec % 60
  if (mm === 0) return ss + 'с'
  return mm + 'м ' + ss + 'с'
}
</script>

<template>
  <div class="scr-dashboard">
    <!-- ═══════════ HEADER ═══════════ -->
    <header class="scr-header">
      <div class="scr-header-inner">
        <div class="scr-header-brand">
          <span class="scr-header-accent"></span>
          <h1 class="scr-header-title">Накладные</h1>
          <p class="scr-header-subtitle">Контур Логистика · автоподписание</p>
        </div>
        <div class="scr-header-stats">
          <div class="scr-counter">
            <span class="scr-counter-value">{{ notes.length }}</span>
            <span class="scr-counter-label">всего</span>
          </div>
        </div>
        <div class="scr-header-actions">
          <button @click="syncNow" class="scr-sync-btn" :class="{ 'scr-sync-btn--active': isUpdating }">
            <span class="scr-sync-dot" :class="{ 'scr-sync-dot--pulse': countdownPulse, 'scr-sync-dot--urgent': countdown <= 2 && !countdownPulse }"></span>
            <span class="scr-sync-text">{{ countdown }}с</span>
          </button>
          <button @click="themeStore.toggle()" class="scr-theme-btn" :title="themeStore.theme === 'dark' ? 'Светлая тема' : 'Тёмная тема'">
            {{ themeStore.theme === 'dark' ? '☀' : '☾' }}
          </button>
        </div>
      </div>
    </header>

    <!-- ═══════════ MAIN CONTENT ═══════════ -->
    <main class="scr-main">
      <!-- Status bar -->
      <div v-if="status" class="scr-status-bar">
        <div class="scr-status-bar__fill" :style="{ width: tickerProgress + '%' }"></div>
        <div class="scr-status-bar__content">
          <div class="scr-status-bar__cell">
            <span class="scr-label">last tic</span>
            <span class="scr-value font-mono">{{ splitDT(status.lastFetchTime).t || '—' }}</span>
          </div>
          <div class="scr-status-bar__cell scr-status-bar__cell--elapsed">
            <span class="scr-label">elapsed</span>
            <span class="scr-value font-mono">
              {{ formatSec(secondsSinceFetch) }}
            </span>
          </div>
        </div>
        <div v-if="status.lastFetchError" class="scr-status-bar__error" style="grid-column: 1 / -1;">
          <span class="scr-error-icon">✕</span>
          <span>{{ status.lastFetchError }}</span>
        </div>
      </div>

      <!-- Signing failures -->
      <div v-if="status?.signingFailures?.length" class="scr-failures">
        <div class="scr-failures-header">
          <span class="scr-failures-icon">⚠</span>
          <span class="scr-failures-title">Критические ошибки · {{ status.signingFailures.length }}</span>
        </div>
        <ul class="scr-failures-list">
          <li v-for="(e, i) in status.signingFailures" :key="i">{{ e }}</li>
        </ul>
      </div>

      <!-- General error -->
      <div v-if="error" class="scr-error-banner">
        <span class="scr-error-icon">✕</span>
        <span>Ошибка загрузки: {{ error }}</span>
      </div>

      <!-- Search -->
      <div class="scr-search-row">
        <div class="scr-search-wrap">
          <svg class="scr-search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <circle cx="11" cy="11" r="8"/>
            <path d="m21 21-4.35-4.35"/>
          </svg>
          <input
            v-model="query"
            placeholder="Номер, отправитель, получатель, водитель…"
            class="scr-search-input"
          />
        </div>
        <span class="scr-results-count">{{ filtered.length }} / {{ notes.length }}</span>
      </div>

      <!-- Empty state -->
      <div v-if="!loading && !filtered.length" class="scr-empty">
        <div class="scr-empty-mark">∅</div>
        <div class="scr-empty-title">Нет данных</div>
        <div class="scr-empty-hint">Накладные появятся после следующего тикера</div>
      </div>

      <!-- Table -->
      <div v-else class="scr-table-wrap">
        <table class="scr-table">
          <colgroup>
            <col class="scr-col-number" />
            <col class="scr-col-route" />
            <col class="scr-col-status" />
            <col class="scr-col-pic" />
          </colgroup>
          <thead>
            <tr>
              <th class="scr-th">Номер / Дата</th>
              <th class="scr-th">Маршрут</th>
              <th class="scr-th">Статус</th>
              <th class="scr-th scr-th--pic">Pic</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(n, idx) in filtered"
              :key="n.number"
              class="scr-row"
              :class="{ 'scr-row--error': n.status === 'failed' }"
              :style="{ animationDelay: (idx * 40) + 'ms' }"
            >
              <!-- Number / Date -->
              <td class="scr-td scr-td--number">
                <div class="scr-row-number">
                  <span class="scr-row-number__id font-mono">{{ n.number }}</span>
                  <span class="scr-row-number__date">{{ n.date || '—' }}</span>
                </div>
                <div class="scr-row-meta">
                  <span class="scr-meta-item">
                    <span class="scr-meta-label">созд.</span>
                    <span class="scr-meta-value font-mono">{{ splitDT(n.createdAt).t || '—' }}</span>
                  </span>
                  <span class="scr-meta-item">
                    <span class="scr-meta-label">подп.</span>
                    <span class="scr-meta-value font-mono">{{ splitDT(signedAt(n)).t || '—' }}</span>
                  </span>
                </div>
              </td>

              <!-- Route -->
              <td class="scr-td scr-td--route">
                <div class="scr-route-driver">
                  <template v-if="n.driver || n.truck">
                    <span v-if="n.driver">{{ n.driver }}</span>
                    <span v-if="n.driver && n.truck" class="scr-separator">·</span>
                    <span v-if="n.truck" class="font-mono">{{ n.truck }}</span>
                  </template>
                  <span v-else class="scr-dash">—</span>
                </div>
                <div class="scr-route-address">
                  <span class="scr-meta-label">Приём:</span>
                  <strong :class="n.receptionAddress ? '' : 'scr-dash'">{{ n.receptionAddress || '—' }}</strong>
                </div>
                <div class="scr-route-address">
                  <span class="scr-meta-label">Доставка:</span>
                  <strong :class="n.deliveryAddress ? '' : 'scr-dash'">{{ n.deliveryAddress || '—' }}</strong>
                </div>
              </td>

              <!-- Status -->
              <td class="scr-td scr-td--status">
                <template v-if="n.status">
                  <span
                    v-if="n.status === 'signed'"
                    class="scr-badge scr-badge--signed"
                  >Sign</span>
                  <span
                    v-else-if="n.status === 'failed'"
                    class="scr-badge scr-badge--failed"
                  >ошибка</span>
                  <span
                    v-else
                    class="scr-badge scr-badge--pending"
                  >{{ n.status }}</span>
                </template>
                <span v-if="n.error" class="scr-row-error">{{ n.error }}</span>
                <span v-if="!n.status && !n.error" class="scr-dash">—</span>
              </td>

              <!-- Screenshots -->
              <td class="scr-td scr-td--pic">
                <template v-if="n.shots?.length">
                  <div class="scr-pic-wrap">
                    <button
                      class="scr-pic-thumb"
                      @click="openCarousel(n, 0)"
                      :title="n.shots[0]"
                    >
                      <img
                        :src="`/screenshots/${n.number}/${n.shots[0]}`"
                        loading="lazy"
                        class="scr-pic-img"
                      />
                    </button>
                    <span v-if="n.shots.length > 1" class="scr-pic-count">{{ n.shots.length - 1 }}</span>
                  </div>
                </template>
                <span v-else class="scr-dash">—</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Last ticker notes -->
      <div v-if="status?.lastNotes?.length" class="scr-ticker-section">
        <details class="scr-ticker-details">
          <summary class="scr-ticker-summary">
            <span class="scr-ticker-summary__label">Последний тикер</span>
            <span class="scr-ticker-summary__count">{{ status.lastNotesCount }} накладных</span>
            <svg class="scr-ticker-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="m6 9 6 6 6-6"/>
            </svg>
          </summary>
          <div class="scr-ticker-list">
            <div v-for="n in status.lastNotes" :key="n.number" class="scr-ticker-item">
              <span class="scr-ticker-item__number font-mono">{{ n.number }}</span>
              <span class="scr-ticker-item__date">от {{ n.date }}</span>
              <span class="scr-ticker-item__route">{{ n.consignor }} → {{ n.consignee }}</span>
              <span class="scr-ticker-item__carrier">{{ n.carrier }}</span>
            </div>
          </div>
        </details>
      </div>
    </main>

    <!-- ═══════════ CAROUSEL OVERLAY ═══════════ -->
    <Teleport to="body">
      <Transition name="scr-carousel">
        <div v-if="showCarousel" class="scr-carousel-overlay" @click.self="closeCarousel">
          <div class="scr-carousel-vignette"></div>
          <button class="scr-carousel-close" @click="closeCarousel">✕</button>
          <button class="scr-carousel-nav scr-carousel-nav--prev" @click="prev">‹</button>
          <button class="scr-carousel-nav scr-carousel-nav--next" @click="next">›</button>
          <div class="scr-carousel-content">
            <div class="scr-carousel-header">
              <span class="scr-carousel-number font-mono">{{ cNumber }}</span>
              <span class="scr-carousel-counter">{{ cIndex + 1 }} / {{ cShots.length }}</span>
              <span class="scr-carousel-filename font-mono truncate">{{ cShots[cIndex] }}</span>
            </div>
            <div class="scr-carousel-image-wrap">
              <img :src="shotUrl(cShots[cIndex])" class="scr-carousel-img" />
            </div>
            <div class="scr-carousel-thumbs">
              <button
                v-for="(s, i) in cShots"
                :key="s"
                class="scr-carousel-thumb"
                :class="{ 'scr-carousel-thumb--active': i === cIndex }"
                @click="cIndex = i"
              >
                <img :src="shotUrl(s)" loading="lazy" class="scr-carousel-thumb-img" />
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- Footer -->
    <footer class="scr-footer">
      <a href="/legacy" class="scr-footer-link">Legacy HTML</a>
      <span class="scr-footer-sep">·</span>
      <span>Обновление каждые 5 секунд</span>
    </footer>
  </div>
</template>

<style scoped>
/* ── CSS Variables (scoped) ─────────────────────────── */
.scr-dashboard {
  --m3-bg: var(--m3-background);
  --m3-surface: var(--m3-surfaceContainer);
  --m3-surfaceLow: var(--m3-surfaceContainerLow);
  --m3-surfaceHigh: var(--m3-surfaceContainerHigh);
  --m3-surfaceHighest: var(--m3-surfaceContainerHighest);
  --m3-onBg: var(--m3-onBackground);
  --m3-onSurface: var(--m3-onSurface);
  --m3-onSurfaceVar: var(--m3-onSurfaceVariant);
  --m3-outline: var(--m3-outline);
  --m3-outlineVar: var(--m3-outlineVariant);
  --m3-primary: var(--m3-primary);
  --m3-onPrimary: var(--m3-onPrimary);
  --m3-primaryContainer: var(--m3-primaryContainer);
  --m3-inverseSurface: var(--m3-inverseSurface);
  --m3-inverseOnSurface: var(--m3-inverseOnSurface);
  --m3-inversePrimary: var(--m3-inversePrimary);
  --m3-error: #f2b8b5;
  --m3-errorContainer: #8c1d18;
  --m3-warning: #f2cc8f;
  --m3-success: #aed581;
  background-color: var(--m3-bg);
  background-image:
    radial-gradient(ellipse 90% 420px at 50% 0%, var(--dash-glow), transparent 70%),
    linear-gradient(var(--dash-grid) 1px, transparent 1px),
    linear-gradient(90deg, var(--dash-grid) 1px, transparent 1px);
  background-size: 100% 480px, 32px 32px, 32px 32px;
  --dash-grid: rgba(0, 0, 0, 0.045);
  --dash-glow: rgba(97, 93, 99, 0.10);
  --dash-orb-a: rgba(124, 132, 255, 0.12);
  --dash-orb-b: rgba(255, 168, 88, 0.09);
  --font-display: var(--font-display);
  --font-mono: var(--font-mono);
  position: relative;
  isolation: isolate;
  max-width: 1200px;
  margin: 0 auto;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}
.dark .scr-dashboard {
  --dash-grid: rgba(255, 255, 255, 0.03);
  --dash-glow: rgba(196, 184, 216, 0.07);
  --dash-orb-a: rgba(148, 138, 255, 0.11);
  --dash-orb-b: rgba(255, 148, 92, 0.06);
}

/* Ambient glow orbs */
.scr-dashboard::before,
.scr-dashboard::after {
  content: '';
  position: fixed;
  inset: -20%;
  z-index: -1;
  pointer-events: none;
}
.scr-dashboard::before {
  background: radial-gradient(560px 420px at 18% 92%, var(--dash-orb-a), transparent 70%);
  animation: scr-orb-drift-a 26s ease-in-out infinite alternate;
}
.scr-dashboard::after {
  background: radial-gradient(640px 480px at 82% 98%, var(--dash-orb-b), transparent 70%);
  animation: scr-orb-drift-b 32s ease-in-out infinite alternate;
}
@keyframes scr-orb-drift-a {
  from { transform: translate3d(0, 0, 0) scale(1); }
  to { transform: translate3d(4%, 6%, 0) scale(1.12); }
}
@keyframes scr-orb-drift-b {
  from { transform: translate3d(0, 0, 0) scale(1.08); }
  to { transform: translate3d(-5%, -4%, 0) scale(1); }
}
@media (prefers-reduced-motion: reduce) {
  .scr-dashboard::before,
  .scr-dashboard::after {
    animation: none;
  }
}

/* ═══════════ HEADER ═══════════ */
.scr-header {
  position: sticky;
  top: 0;
  z-index: 10;
  background: color-mix(in srgb, var(--m3-bg) 82%, transparent);
  -webkit-backdrop-filter: blur(8px);
  backdrop-filter: blur(8px);
  border-bottom: 1px solid var(--m3-outlineVar);
}
.scr-header-inner {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: 1rem;
  padding: 0.75rem 1.5rem;
}
.scr-header-brand {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.scr-header-accent {
  width: 3px;
  height: 28px;
  background: var(--m3-onBg);
  flex-shrink: 0;
}
.scr-header-title {
  font-family: var(--font-display);
  font-size: 1.125rem;
  font-weight: 700;
  letter-spacing: -0.02em;
  color: var(--m3-onBg);
  line-height: 1;
}
.scr-header-subtitle {
  font-size: 0.65rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--m3-onSurfaceVar);
  margin-top: 0.15rem;
}
.scr-header-stats {
  display: flex;
  justify-content: center;
}
.scr-counter {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 0.4rem 1.25rem;
  background: var(--m3-surfaceLow);
  border: 1px solid var(--m3-outlineVar);
}
.scr-counter-value {
  font-family: var(--font-mono);
  font-size: 1.5rem;
  font-weight: 700;
  color: var(--m3-onBg);
  line-height: 1;
}
.scr-counter-label {
  font-size: 0.6rem;
  text-transform: uppercase;
  letter-spacing: 0.12em;
  color: var(--m3-onSurfaceVar);
  margin-top: 0.2rem;
}
.scr-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.5rem;
}

/* Sync button */
.scr-sync-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  background: none;
  border: none;
  cursor: pointer;
  padding: 0.3rem 0.5rem;
  color: var(--m3-onSurfaceVar);
  transition: color 0.15s ease;
}
.scr-sync-btn:hover {
  color: var(--m3-onBg);
}
.scr-sync-btn--active {
  color: var(--m3-onBg);
}
.scr-sync-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--m3-onSurfaceVar);
  flex-shrink: 0;
  position: relative;
}
.scr-sync-dot--pulse::after {
  content: '';
  position: absolute;
  inset: -4px;
  border-radius: 50%;
  border: 1px solid currentColor;
  animation: scr-dot-ping 0.85s cubic-bezier(0, 0, 0.2, 1) infinite;
}
.scr-sync-dot--urgent {
  background: var(--m3-error);
  animation: scr-dot-blink 0.7s step-end infinite;
}
.scr-sync-text {
  font-family: var(--font-mono);
  font-size: 0.7rem;
  font-variant-numeric: tabular-nums;
}

/* Theme button */
.scr-theme-btn {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--m3-surfaceLow);
  border: 1px solid var(--m3-outlineVar);
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.9rem;
  color: var(--m3-onSurfaceVar);
  transition: all 0.15s ease;
}
.scr-theme-btn:hover {
  border-color: var(--m3-onBg);
  color: var(--m3-onBg);
}

/* ═══════════ MAIN ═══════════ */
.scr-main {
  flex: 1;
  padding: 1rem 1.5rem 2rem;
}

/* ── Status bar ── */
.scr-status-bar {
  position: relative;
  display: grid;
  grid-template-columns: 1fr auto;
  align-items: stretch;
  background: var(--m3-surfaceContainer);
  border: 1px solid var(--m3-outlineVar);
  border-radius: 6px;
  overflow: hidden;
  margin-bottom: 0.5rem;
}
.scr-status-bar__fill {
  position: absolute;
  bottom: 0;
  left: 0;
  height: 3px;
  width: 0%;
  min-width: 0;
  background: var(--m3-secondary);
  transition: width 1s linear;
  pointer-events: none;
}
.scr-status-bar__content {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: baseline;
  gap: 2rem;
  padding: 0.5rem 1rem;
  flex: 1;
  min-width: 0;
}
.scr-status-bar__cell {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
}
.scr-status-bar__cell--elapsed {
  padding-left: 1rem;
  border-left: 1px solid var(--m3-outlineVar);
}
.scr-label {
  font-size: 0.6rem;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  color: var(--m3-onSurfaceVar);
  font-weight: 500;
}
.scr-value {
  font-family: var(--font-mono);
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--m3-onBg);
  font-variant-numeric: tabular-nums;
}

.scr-status-bar__error {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.4rem 1rem;
  border-top: 1px solid var(--m3-outlineVar);
  background: rgba(140, 29, 24, 0.15);
  color: var(--m3-error);
  font-size: 0.75rem;
}

html:not(.dark) .scr-status-bar__error {
  color: #b3261e !important;
  background: rgba(140, 29, 24, 0.08) !important;
}

/* ── Failures ── */
.scr-failures {
  border: 1px solid var(--m3-outlineVar);
  border-left: 3px solid var(--m3-error);
  background: var(--m3-surfaceContainer);
  border-radius: 6px;
  padding: 0.75rem 1rem;
  margin-bottom: 0.5rem;
}
.scr-failures-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--m3-onBg);
  margin-bottom: 0.35rem;
}
.scr-failures-icon {
  color: var(--m3-error);
}
.scr-failures-list {
  margin: 0;
  padding-left: 1.25rem;
  font-size: 0.8rem;
  color: var(--m3-onSurfaceVar);
  line-height: 1.6;
}
.scr-failures-list li {
  margin-bottom: 0.1rem;
}

/* ── Error banner ── */
.scr-error-banner {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  border: 1px solid var(--m3-outlineVar);
  border-left: 3px solid var(--m3-error);
  background: rgba(140, 29, 24, 0.15);
  border-radius: 6px;
  padding: 0.6rem 1rem;
  margin-bottom: 0.5rem;
  font-size: 0.8rem;
  color: var(--m3-error);
}

html:not(.dark) .scr-error-banner {
  color: #b3261e !important;
  background: rgba(140, 29, 24, 0.08) !important;
  border-left-color: #b3261e !important;
}

/* ── Search ── */
.scr-search-row {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-bottom: 0.5rem;
}
.scr-search-wrap {
  position: relative;
  flex: 1;
  max-width: 480px;
}
.scr-search-icon {
  position: absolute;
  left: 0.75rem;
  top: 50%;
  transform: translateY(-50%);
  width: 14px;
  height: 14px;
  color: var(--m3-onSurfaceVar);
  pointer-events: none;
}
.scr-search-input {
  width: 100%;
  height: 34px;
  padding: 0 0.75rem 0 2rem;
  background: var(--m3-surfaceLow);
  border: 1px solid var(--m3-outlineVar);
  border-radius: 6px;
  font-family: var(--font-display);
  font-size: 0.8rem;
  color: var(--m3-onBg);
  outline: none;
  transition: border-color 0.15s ease;
}
.scr-search-input::placeholder {
  color: var(--m3-onSurfaceVar);
}
.scr-search-input:focus {
  border-color: var(--m3-onBg);
}
.scr-results-count {
  font-family: var(--font-mono);
  font-size: 0.7rem;
  color: var(--m3-onSurfaceVar);
  font-variant-numeric: tabular-nums;
}

/* ── Empty state ── */
.scr-empty {
  text-align: center;
  padding: 3rem 1.5rem;
  border: 1px dashed var(--m3-outlineVar);
  border-radius: 8px;
  background: var(--m3-surfaceLow);
}
.scr-empty-mark {
  font-size: 2.5rem;
  color: var(--m3-onSurfaceVar);
  line-height: 1;
  animation: scr-empty-breathe 2.4s ease-in-out infinite;
}
@keyframes scr-empty-breathe {
  0%, 100% { opacity: 0.5; transform: scale(1); }
  50% { opacity: 1; transform: scale(1.06); }
}
.scr-empty-title {
  margin-top: 0.75rem;
  font-size: 1rem;
  font-weight: 600;
  color: var(--m3-onBg);
}
.scr-empty-hint {
  margin-top: 0.35rem;
  font-size: 0.8rem;
  color: var(--m3-onSurfaceVar);
}

/* ── Table ── */
.scr-table-wrap {
  border: 1px solid var(--m3-outlineVar);
  border-radius: 8px;
  overflow-x: auto;
  background: var(--m3-bg);
}
.scr-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.8rem;
}
.scr-col-number { width: 28%; }
.scr-col-route { width: 40%; }
.scr-col-status { width: 20%; }
.scr-col-pic { width: 12%; }
.scr-th.scr-th--pic {
  text-align: center;
}

.scr-th {
  text-align: left;
  font-size: 0.6rem;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  font-weight: 600;
  color: var(--m3-onSurfaceVar);
  background: var(--m3-surfaceLow);
  border-bottom: 1px solid var(--m3-outlineVar);
  padding: 0.6rem 0.75rem;
  white-space: nowrap;
  vertical-align: bottom;
}
.scr-table tbody tr {
  border-bottom: 1px solid var(--m3-outlineVar);
  transition: background-color 0.15s ease-out;
  animation: scr-row-in 0.4s ease-out both;
}
.scr-table tbody tr:last-child {
  border-bottom: none;
}
.scr-table tbody tr:hover {
  background: var(--m3-surfaceLow);
  box-shadow: inset 2px 0 0 var(--m3-secondary);
}
.scr-td {
  padding: 0.6rem 0.75rem;
  vertical-align: top;
  color: var(--m3-onBg);
}
.scr-td--pic {
  padding: 0.6rem 0.75rem;
  vertical-align: middle !important;
  text-align: center;
}
.scr-td--status {
  vertical-align: middle !important;
}
.scr-td--number {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}
.scr-row-number {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
}
.scr-row-number__id {
  font-family: var(--font-mono);
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--m3-onBg);
  word-break: break-all;
}
.scr-row-number__date {
  font-size: 0.7rem;
  color: var(--m3-onSurfaceVar);
  word-break: break-all;
}
.scr-row-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
}
.scr-meta-item {
  display: flex;
  align-items: baseline;
  gap: 0.3rem;
  font-size: 0.65rem;
}
.scr-meta-label {
  text-transform: uppercase;
  letter-spacing: 0.06em;
  font-weight: 500;
  color: var(--m3-onSurfaceVar);
}
.scr-meta-value {
  font-family: var(--font-mono);
  font-size: 0.65rem;
  color: var(--m3-onSurfaceVar);
  word-break: break-all;
}

/* Route */
.scr-route-driver {
  font-size: 0.7rem;
  color: var(--m3-onSurfaceVar);
  word-break: break-all;
}
.scr-separator {
  color: var(--m3-onSurfaceVar);
}
.scr-route-address {
  font-size: 0.7rem;
  color: var(--m3-onSurfaceVar);
  word-break: break-all;
}
.scr-dash {
  color: var(--m3-onSurfaceVar) !important;
  font-weight: 400 !important;
}

/* Status badges */
.scr-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 22px;
  padding: 0 0.5rem;
  font-size: 0.65rem;
  font-weight: 600;
  letter-spacing: 0.02em;
  border-radius: 3px;
  white-space: nowrap;
}
.scr-badge--signed {
  background: var(--m3-surfaceHigh);
  color: var(--m3-onSurfaceVar);
  border: 1px solid var(--m3-outlineVar);
}
.scr-badge--failed {
  background: rgba(220, 38, 38, 0.08);
  color: var(--m3-error);
  border: 1px solid rgba(220, 38, 38, 0.2);
}
.scr-badge--pending {
  background: var(--m3-surfaceLow);
  color: var(--m3-onSurfaceVar);
  border: 1px solid var(--m3-outlineVar);
}
.scr-row-error {
  display: block;
  font-size: 0.7rem;
  color: var(--m3-error);
  margin-top: 0.25rem;
  word-break: break-all;
}

html:not(.dark) .scr-row-error {
  color: #b3261e !important;
}

/* Pic column */
.scr-pic-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.2rem;
  width: fit-content;
  margin: 0 auto;
}
.scr-pic-thumb {
  width: 36px;
  height: 24px;
  border: 1px solid var(--m3-outlineVar);
  border-radius: 3px;
  overflow: hidden;
  background: var(--m3-surfaceLow);
  cursor: pointer;
  padding: 0;
}
.scr-pic-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.scr-pic-count {
  font-family: var(--font-mono);
  font-size: 0.6rem;
  color: var(--m3-onSurfaceVar);
}

/* Row error accent */
.scr-row--error td:first-child {
  border-left: 2px solid var(--m3-error);
}

/* ── Light theme: make red elements readable ── */
html:not(.dark) .scr-sync-dot--urgent,
html:not(.dark) .scr-status-bar__error,
html:not(.dark) .scr-error-banner,
html:not(.dark) .scr-failures,
html:not(.dark) .scr-failures-icon,
html:not(.dark) .scr-badge--failed,
html:not(.dark) .scr-row-error {
  color: #b3261e !important;
}
html:not(.dark) .scr-sync-dot--urgent {
  background: #b3261e !important;
}
html:not(.dark) .scr-failures,
html:not(.dark) .scr-failures-icon,
html:not(.dark) .scr-error-banner {
  border-left-color: #b3261e !important;
}
html:not(.dark) .scr-row--error td:first-child {
  border-left-color: #b3261e !important;
}

/* ═══════════ TICKER SECTION ═══════════ */
.scr-ticker-section {
  margin-top: 0.5rem;
}
.scr-ticker-details {
  border: 1px solid var(--m3-outlineVar);
  border-radius: 6px;
  background: var(--m3-bg);
  overflow: hidden;
}
.scr-ticker-summary {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.6rem 1rem;
  background: var(--m3-surfaceLow);
  border-bottom: 1px solid var(--m3-outlineVar);
  cursor: pointer;
  list-style: none;
  user-select: none;
}
.scr-ticker-summary::-webkit-details-marker {
  display: none;
}
.scr-ticker-summary__label {
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--m3-onBg);
}
.scr-ticker-summary__count {
  font-family: var(--font-mono);
  font-size: 0.65rem;
  color: var(--m3-onSurfaceVar);
}
.scr-ticker-chevron {
  width: 12px;
  height: 12px;
  margin-left: auto;
  color: var(--m3-onSurfaceVar);
  transition: transform 0.2s ease;
}
.scr-ticker-details[open] .scr-ticker-chevron {
  transform: rotate(180deg);
}
.scr-ticker-list {
  padding: 0.5rem 1rem;
}
.scr-ticker-item {
  display: flex;
  flex-wrap: wrap;
  gap: 0.3rem 0.75rem;
  padding: 0.35rem 0;
  border-bottom: 1px solid var(--m3-outlineVar);
  font-size: 0.7rem;
  color: var(--m3-onSurfaceVar);
}
.scr-ticker-item:last-child {
  border-bottom: none;
}
.scr-ticker-item__number {
  font-weight: 600;
  color: var(--m3-onBg);
}
.scr-ticker-item__date {
  color: var(--m3-onSurfaceVar);
}
.scr-ticker-item__route {
  color: var(--m3-onSurfaceVar);
}
.scr-ticker-item__carrier {
  color: var(--m3-onSurfaceVar);
}

/* ═══════════ CAROUSEL ═══════════ */
.scr-carousel-overlay {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.85);
  backdrop-filter: blur(4px);
  padding: 2rem;
}
.scr-carousel-vignette {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: radial-gradient(ellipse at center, transparent 40%, rgba(0,0,0,0.6) 100%);
}
.scr-carousel-close {
  position: absolute;
  top: 1rem;
  right: 1rem;
  z-index: 2;
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: 8px;
  color: rgba(255, 255, 255, 0.7);
  font-size: 1rem;
  cursor: pointer;
  transition: all 0.15s ease;
}
.scr-carousel-close:hover {
  background: rgba(255, 255, 255, 0.2);
  color: #fff;
}
.scr-carousel-nav {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  z-index: 2;
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 50%;
  color: rgba(255, 255, 255, 0.7);
  font-size: 1.5rem;
  cursor: pointer;
  transition: all 0.15s ease;
}
.scr-carousel-nav:hover {
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
}
.scr-carousel-nav--prev { left: 1.5rem; }
.scr-carousel-nav--next { right: 1.5rem; }

.scr-carousel-content {
  position: relative;
  z-index: 1;
  max-width: min(1100px, 94vw);
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  background: var(--m3-bg);
  border-radius: 10px;
  overflow: hidden;
  border: 1px solid var(--m3-outlineVar);
}
.scr-carousel-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.6rem 1rem;
  border-bottom: 1px solid var(--m3-outlineVar);
  background: var(--m3-surfaceLow);
  font-size: 0.75rem;
  white-space: nowrap;
  overflow: hidden;
}
.scr-carousel-number {
  font-weight: 600;
  color: var(--m3-onBg);
}
.scr-carousel-counter {
  color: var(--m3-onSurfaceVar);
}
.scr-carousel-filename {
  margin-left: auto;
  color: var(--m3-onSurfaceVar);
  font-size: 0.65rem;
  overflow: hidden;
  text-overflow: ellipsis;
}
.scr-carousel-image-wrap {
  background: #000;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 300px;
  max-height: 60vh;
}
.scr-carousel-img {
  max-width: 100%;
  max-height: 60vh;
  object-fit: contain;
}
.scr-carousel-thumbs {
  display: flex;
  gap: 0.4rem;
  padding: 0.5rem 0.75rem;
  overflow-x: auto;
  border-top: 1px solid var(--m3-outlineVar);
  background: var(--m3-bg);
}
.scr-carousel-thumb {
  flex-shrink: 0;
  width: 64px;
  height: 44px;
  border: 1px solid var(--m3-outlineVar);
  border-radius: 4px;
  overflow: hidden;
  cursor: pointer;
  padding: 0;
  transition: all 0.15s ease;
  opacity: 0.5;
}
.scr-carousel-thumb--active {
  border-color: var(--m3-onBg);
  opacity: 1;
  box-shadow: 0 0 0 2px rgba(0, 0, 0, 0.1);
}
.dark .scr-carousel-thumb--active {
  box-shadow: 0 0 0 2px rgba(255, 255, 255, 0.15);
}
.scr-carousel-thumb-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

/* Carousel transitions */
.scr-carousel-enter-active {
  animation: scr-carousel-in 0.25s cubic-bezier(0.16, 1, 0.3, 1);
}
.scr-carousel-leave-active {
  animation: scr-carousel-out 0.2s ease-in;
}
@keyframes scr-carousel-in {
  from { opacity: 0; transform: scale(0.97); }
  to { opacity: 1; transform: scale(1); }
}
@keyframes scr-carousel-out {
  from { opacity: 1; transform: scale(1); }
  to { opacity: 0; transform: scale(0.97); }
}

/* ═══════════ FOOTER ═══════════ */
.scr-footer {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 1.5rem 1rem 2rem;
  font-size: 0.7rem;
  color: var(--m3-onSurfaceVar);
}
.scr-footer-link {
  color: var(--m3-onSurfaceVar);
  text-decoration: underline;
  text-underline-offset: 2px;
  transition: color 0.15s ease;
}
.scr-footer-link:hover {
  color: var(--m3-onBg);
}
.scr-footer-sep {
  color: var(--m3-outlineVar);
}

/* ═══════════ ANIMATIONS ═══════════ */
@keyframes scr-row-in {
  from {
    opacity: 0;
    transform: translateY(4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
@keyframes scr-dot-ping {
  0% { transform: scale(1); opacity: 0.45; }
  100% { transform: scale(2.4); opacity: 0; }
}
@keyframes scr-dot-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.35; }
}

/* ═══════════ RESPONSIVE ═══════════ */
@media (max-width: 768px) {
  .scr-header-inner {
    grid-template-columns: auto 1fr auto;
    gap: 0.3rem;
    padding: 0.5rem 1rem;
  }
  .scr-header-brand {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.15rem;
  }
  .scr-header-accent {
    display: none;
  }
  .scr-header-title {
    font-size: 0.9rem;
  }
  .scr-header-subtitle {
    font-size: 0.4rem;
    letter-spacing: 0.03em;
    line-height: 1.1;
  }
  .scr-header-accent {
    width: 2px;
    height: 14px;
    align-self: flex-start;
    margin-top: 0.1rem;
  }
  .scr-header-stats {
    justify-self: center;
  }
  .scr-header-actions {
    justify-self: end;
  }
  .scr-counter {
    padding: 0.2rem 0.6rem;
  }
  .scr-counter-value {
    font-size: 1rem;
  }
  .scr-counter-label {
    font-size: 0.45rem;
  }
  .scr-sync-text {
    font-size: 0.6rem;
  }
  .scr-theme-btn {
    width: 26px;
    height: 26px;
    font-size: 0.75rem;
  }
  .scr-header-actions {
    gap: 0.3rem;
  }
  .scr-main {
    padding: 0.75rem 1rem 1.5rem;
  }
  .scr-status-bar__content {
    display: flex !important;
    justify-content: space-between;
    padding: 0.3rem 0.75rem;
  }
  .scr-status-bar__cell {
    font-size: 0.65rem;
  }
  .scr-status-bar__cell--elapsed {
    text-align: right;
    border-left: 1px solid var(--m3-outlineVar);
    padding-left: 1rem;
  }
  .scr-label {
    font-size: 0.5rem;
  }
  .scr-value {
    font-size: 0.7rem;
  }
  .scr-carousel-overlay {
    padding: 0.5rem;
  }
  .scr-carousel-content {
    max-width: calc(100vw - 6rem);
  }
  .scr-carousel-nav {
    width: 38px;
    height: 38px;
    font-size: 1.2rem;
    background: rgba(255, 255, 255, 0.18);
    border-color: rgba(255, 255, 255, 0.35);
    color: #fff;
    box-shadow: 0 2px 10px rgba(0, 0, 0, 0.45);
  }
  .scr-carousel-nav--prev { left: 0.5rem; }
  .scr-carousel-nav--next { right: 0.5rem; }
}
</style>
