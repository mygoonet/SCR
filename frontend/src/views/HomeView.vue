<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'

const notes = ref([])
const status = ref(null)
const loading = ref(true)
const error = ref('')
const query = ref('')
const isUpdating = ref(false)
let timer = null
let flashTimer = null

async function fetchData() {
  isUpdating.value = true
  try {
    const [r1, r2] = await Promise.all([fetch('/api/notes'), fetch('/api/status')])
    if (!r1.ok) throw new Error('notes ' + r1.status)
    notes.value = await r1.json()
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
})
onUnmounted(() => timer && clearInterval(timer))

function signedAt(n) {
  if (n.signedAt?.length) return n.signedAt.join(', ')
  return n.processedAt || '—'
}

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return notes.value
  return notes.value.filter(n =>
    n.number.toLowerCase().includes(q) ||
    (n.consignor && n.consignor.toLowerCase().includes(q)) ||
    (n.consignee && n.consignee.toLowerCase().includes(q)) ||
    (n.driver && n.driver.toLowerCase().includes(q)) ||
    (n.truck && n.truck.toLowerCase().includes(q)) ||
    (n.status && n.status.toLowerCase().includes(q))
  )
})

function statusClass(s) {
  if (s === 'signed') return 'badge badge--ok'
  if (s === 'failed') return 'badge badge--err'
  return 'badge badge--wait'
}

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
  const m = status.value?.minutesSinceFetch
  if (m == null || m < 0) return 0
  const intervalMin = 6
  return Math.min(100, Math.max(0, (m / intervalMin) * 100))
})
</script>

<template>
  <div class="page">
    <!-- header -->
    <header class="header">
      <div class="header__inner">
        <div>
          <h1 class="title">Накладные</h1>
          <p class="subtitle">Контур Логистика · автоматическое подписание</p>
        </div>
        <div class="header__right">
          <div class="count">
            <span class="count__num">{{ notes.length }}</span>
            <span class="count__label">всего</span>
          </div>
          <span class="sync" :class="{ 'sync--loading': isUpdating }">
            <span class="dot" :class="{ 'dot--pulse': isUpdating }"></span>
            каждые 5 сек
          </span>
        </div>
      </div>
    </header>

    <!-- status bar -->
    <div class="statusbar" v-if="status">
      <div class="statusbar__fill" :style="{ width: tickerProgress + '%' }"></div>
      <div class="statusbar__item">
        <span class="k">Тикер</span>
        <span class="dt" v-if="status.lastFetchTime">
          <span class="dt__date">{{ splitDT(status.lastFetchTime).d }}</span>
          <span class="dt__time">{{ splitDT(status.lastFetchTime).t }}</span>
        </span>
        <span v-else class="muted">—</span>
        <span class="muted" v-if="status.minutesSinceFetch != null">· {{ status.minutesSinceFetch }} мин назад</span>
      </div>
      <div class="statusbar__item">
        <span class="k">Сейчас</span>
        <span class="dt">
          <span class="dt__date">{{ splitDT(status.now).d }}</span>
          <span class="dt__time">{{ splitDT(status.now).t }}</span>
        </span>
      </div>
      <div class="statusbar__item" v-if="status.lastFetchError">
        <span class="k" style="color:#991b1b">Ошибка</span>
        <span class="v" style="color:#991b1b">{{ status.lastFetchError }}</span>
      </div>
    </div>

    <!-- errors -->
    <div v-if="status?.signingFailures?.length" class="alert">
      <div class="alert__title">Критические ошибки · {{ status.signingFailures.length }}</div>
      <ul class="alert__list">
        <li v-for="(e,i) in status.signingFailures" :key="i">{{ e }}</li>
      </ul>
    </div>

    <div v-if="error" class="alert alert--error">
      Ошибка загрузки: {{ error }}
    </div>

    <!-- toolbar -->
    <div class="toolbar">
      <input v-model="query" placeholder="Поиск по номеру, контрагенту, статусу…" class="search" />
      <span class="toolbar__hint">{{ filtered.length }} из {{ notes.length }}</span>
    </div>

    <!-- empty -->
    <div v-if="!loading && !filtered.length" class="empty">
      <div class="empty__icon">—</div>
      <div class="empty__text">Нет данных</div>
      <div class="empty__sub">Накладные появятся после следующего тикера</div>
    </div>

    <!-- table -->
    <div v-else class="tablewrap">
      <table class="table">
        <thead>
          <tr>
            <th>Номер</th>
            <th>Создано</th>
            <th>Подписано</th>
            <th>Дата</th>
            <th>Маршрут</th>
            <th>Статус</th>
            <th>Ошибка</th>
            <th>Скриншоты</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="n in filtered" :key="n.number">
            <td class="mono strong">{{ n.number }}</td>
            <td class="muted">{{ n.createdAt || '—' }}</td>
            <td class="muted">{{ signedAt(n) }}</td>
            <td>{{ n.date || '—' }}</td>
            <td>
              <div class="route-col">
                <div class="route-line"><span class="k">Отправитель:</span> {{ n.consignor || '—' }}</div>
                <div class="route-line"><span class="k">Получатель:</span> {{ n.consignee || '—' }}</div>
                <div v-if="n.driver || n.truck" class="route__meta" style="margin-top:6px">
                  <span v-if="n.driver">{{ n.driver }}</span>
                  <span v-if="n.driver && n.truck"> · </span>
                  <span v-if="n.truck" class="mono">{{ n.truck }}</span>
                </div>
                <div class="route__meta route__meta--addr">
                  {{ n.consignorAddress || '—' }} → {{ n.consigneeAddress || '—' }}
                </div>
              </div>
            </td>
            <td><span :class="statusClass(n.status)">{{ n.status || '—' }}</span></td>
            <td class="err">{{ n.error || '' }}</td>
            <td>
              <div class="shots">
                <button v-for="(s,i) in n.shots" :key="s" class="shot" @click="openCarousel(n,i)" :title="s">
                  <img :src="`/screenshots/${n.number}/${s}`" loading="lazy" alt="" />
                </button>
                <span v-if="!n.shots?.length" class="muted">—</span>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- ticker notes -->
    <div v-if="status?.lastNotes?.length" class="card">
      <div class="card__head">
        <h3>Последний тикер</h3>
        <span class="muted">{{ status.lastNotesCount }} накладных</span>
      </div>
      <div class="card__body mono">
        <div v-for="n in status.lastNotes" :key="n.number" class="ticker-row">
          <span class="strong">{{ n.number }}</span>
          <span class="muted">от {{ n.date }}</span>
          <span>{{ n.consignor }} → {{ n.consignee }}</span>
          <span class="muted">{{ n.carrier }}</span>
        </div>
      </div>
    </div>

    <!-- carousel modal -->
    <Teleport to="body">
      <div v-if="showCarousel" class="overlay" @click.self="closeCarousel">
        <div class="carousel">
          <div class="carousel__top">
            <span class="mono strong">{{ cNumber }} · {{ cIndex + 1 }} / {{ cShots.length }}</span>
            <span class="mono muted" style="font-size:11px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:40vw">{{ cShots[cIndex] }}</span>
            <button class="btn btn--close" @click="closeCarousel">✕</button>
          </div>
          <div class="carousel__main">
            <button class="nav nav--left" @click="prev" aria-label="prev">‹</button>
            <img :src="shotUrl(cShots[cIndex])" class="carousel__img" alt="" />
            <button class="nav nav--right" @click="next" aria-label="next">›</button>
          </div>
          <div class="thumbs">
            <button v-for="(s,i) in cShots" :key="s" class="thumb" :class="{ 'thumb--active': i === cIndex }" @click="cIndex = i">
              <img :src="shotUrl(s)" loading="lazy" alt="" />
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <footer class="footer">
      <a href="/">Legacy HTML</a>
      <span class="muted">·</span>
      <span class="muted">Обновление данных каждые 5 секунд</span>
    </footer>
  </div>
</template>

<style scoped>
.page {
  max-width: 1280px;
  margin: 0 auto;
  padding: 0 24px 40px;
}

/* header */
.header {
  border-bottom: 1px solid var(--border);
  margin: 0 -24px;
  padding: 28px 24px 20px;
  background: #fff;
  position: sticky;
  top: 0;
  z-index: 10;
}
.header__inner {
  max-width: 1280px;
  margin: 0 auto;
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 16px;
}
.title {
  font-size: 22px;
  font-weight: 600;
  letter-spacing: -0.02em;
  color: #0a0a0a;
  line-height: 1.1;
}
.subtitle {
  margin-top: 4px;
  font-size: 13px;
  color: var(--text-secondary);
}
.header__right {
  display: flex;
  align-items: center;
  gap: 16px;
}
.count {
  text-align: right;
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 8px 14px;
  background: var(--surface);
}
.count__num {
  display: block;
  font-size: 20px;
  font-weight: 600;
  line-height: 1;
  color: #0a0a0a;
}
.count__label {
  font-size: 11px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--text-muted);
}
.sync {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-muted);
  white-space: nowrap;
  min-width: 128px;
  justify-content: flex-start;
}
.sync--loading {
  color: #0a0a0a;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #9ca3af;
  position: relative;
  flex-shrink: 0;
}
.dot--pulse {
  background: #0a0a0a;
  animation: dot-blink 0.7s step-end infinite;
}
.dot--pulse::after {
  content: '';
  position: absolute;
  inset: -6px;
  border-radius: inherit;
  border: 1px solid #0a0a0a;
  animation: dot-ping 0.85s cubic-bezier(0,0,0.2,1) infinite;
}
@keyframes dot-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.35; }
}
@keyframes dot-ping {
  0% { transform: scale(1); opacity: 0.45; }
  100% { transform: scale(2.4); opacity: 0; }
}

/* statusbar */
.statusbar {
  margin-top: 16px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
  padding: 10px 14px;
  background: #fafafa;
  border: 1px solid var(--border);
  border-radius: 10px;
  position: relative;
  overflow: hidden;
}
.statusbar__fill {
  position: absolute;
  inset: 0 auto 0 0;
  background: #e5e7eb;
  transition: width 1s linear;
  pointer-events: none;
}
.statusbar__item {
  position: relative;
  z-index: 1;
}
.statusbar__item {
  display: flex;
  gap: 8px;
  align-items: baseline;
  font-size: 13px;
}
.k {
  font-size: 11px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--text-muted);
  font-weight: 500;
}
.v {
  color: #0a0a0a;
}
.dt {
  display: inline-flex;
  gap: 6px;
  align-items: baseline;
}
.dt__date {
  color: #9ca3af;
  font-size: 13px;
  font-weight: 400;
}
.dt__time {
  color: #0a0a0a;
  font-weight: 700;
  font-size: 13px;
  letter-spacing: -0.01em;
}
.muted {
  color: var(--text-muted);
}

/* alert */
.alert {
  margin-top: 12px;
  border: 1px solid #e5e7eb;
  border-left: 3px solid #0a0a0a;
  background: #fafafa;
  border-radius: 10px;
  padding: 12px 14px;
}
.alert--error {
  border-left-color: #dc2626;
  background: #fef2f2;
  color: #7f1d1d;
}
.alert__title {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
  color: #0a0a0a;
  margin-bottom: 6px;
}
.alert__list {
  margin-left: 16px;
  font-size: 13px;
  color: #52525b;
}
.alert__list li + li {
  margin-top: 2px;
}

/* toolbar */
.toolbar {
  margin-top: 16px;
  display: flex;
  align-items: center;
  gap: 12px;
}
.search {
  flex: 1;
  max-width: 420px;
  height: 36px;
  padding: 0 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: #fff;
  font-size: 13px;
  color: #0a0a0a;
  outline: none;
}
.search::placeholder {
  color: var(--text-muted);
}
.search:focus {
  border-color: #0a0a0a;
  box-shadow: 0 0 0 3px rgba(10, 10, 10, 0.06);
}
.toolbar__hint {
  font-size: 12px;
  color: var(--text-muted);
}

/* empty */
.empty {
  margin-top: 24px;
  border: 1px dashed var(--border);
  border-radius: 12px;
  padding: 40px 24px;
  text-align: center;
  background: #fafafa;
}
.empty__icon {
  font-size: 20px;
  color: var(--text-muted);
}
.empty__text {
  margin-top: 8px;
  font-weight: 600;
  color: #0a0a0a;
}
.empty__sub {
  margin-top: 4px;
  font-size: 13px;
  color: var(--text-secondary);
}

/* table */
.tablewrap {
  margin-top: 16px;
  border: 1px solid var(--border);
  border-radius: 12px;
  overflow: hidden;
  background: #fff;
}
.table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.table thead th {
  text-align: left;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--text-muted);
  background: #fafafa;
  border-bottom: 1px solid var(--border);
  padding: 10px 12px;
  white-space: nowrap;
}
.table tbody td {
  padding: 10px 12px;
  border-bottom: 1px solid #f0f0f0;
  vertical-align: middle;
}
.table tbody tr:last-child td {
  border-bottom: none;
}
.table tbody tr:hover {
  background: #fafafa;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12.5px;
}
.strong {
  font-weight: 600;
  color: #0a0a0a;
}
.err {
  color: #991b1b;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.route-col {
  max-width: 340px;
  line-height: 1.35;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.route-line {
  word-break: break-word;
  white-space: normal;
  font-size: 13px;
  color: #0a0a0a;
}
.route__arrow {
  color: var(--text-muted);
  flex-shrink: 0;
}
.route__meta {
  margin-top: 3px;
  font-size: 11px;
  color: var(--text-muted);
  max-width: 320px;
  white-space: normal;
  word-break: break-word;
  line-height: 1.35;
}
.route__meta--addr {
  color: #9ca3af;
  margin-top: 2px;
}
.badge {
  display: inline-flex;
  align-items: center;
  height: 22px;
  padding: 0 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.02em;
  border: 1px solid var(--border);
  background: #fff;
  color: #0a0a0a;
  white-space: nowrap;
}
.badge--ok {
  background: #0a0a0a;
  color: #fff;
  border-color: #0a0a0a;
}
.badge--err {
  background: #fff;
  color: #991b1b;
  border-color: #fecaca;
}
.badge--wait {
  background: #fafafa;
  color: #52525b;
}
.shots {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  max-width: 220px;
}
.shot {
  display: block;
  width: 64px;
  height: 42px;
  border: 1px solid var(--border);
  border-radius: 6px;
  overflow: hidden;
  background: #f5f5f5;
  cursor: pointer;
  padding: 0;
}
.shot img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.shot:hover {
  border-color: #0a0a0a;
}

/* card */
.card {
  margin-top: 16px;
  border: 1px solid var(--border);
  border-radius: 12px;
  overflow: hidden;
  background: #fff;
}
.card__head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  padding: 12px 14px;
  border-bottom: 1px solid var(--border);
  background: #fafafa;
}
.card__head h3 {
  font-size: 13px;
  font-weight: 600;
  color: #0a0a0a;
}
.card__body {
  padding: 8px 14px;
}
.ticker-row {
  display: flex;
  gap: 12px;
  padding: 6px 0;
  border-bottom: 1px solid #f5f5f5;
  font-size: 12.5px;
  flex-wrap: wrap;
}
.ticker-row:last-child {
  border-bottom: none;
}

/* carousel */
.overlay {
  position: fixed;
  inset: 0;
  background: rgba(10, 10, 10, 0.72);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
  padding: 24px;
}
.carousel {
  background: #fff;
  border-radius: 12px;
  overflow: hidden;
  width: min(1100px, 96vw);
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  border: 1px solid #e5e7eb;
}
.carousel__top {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--border);
  background: #fafafa;
}
.btn--close {
  margin-left: auto;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  border: 1px solid var(--border);
  background: #fff;
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
}
.btn--close:hover { border-color: #0a0a0a; }
.carousel__main {
  position: relative;
  background: #0a0a0a;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 320px;
  max-height: 62vh;
}
.carousel__img {
  max-width: 100%;
  max-height: 62vh;
  object-fit: contain;
  display: block;
}
.nav {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  width: 36px;
  height: 36px;
  border-radius: 999px;
  border: 1px solid rgba(255,255,255,0.3);
  background: rgba(255,255,255,0.9);
  color: #0a0a0a;
  font-size: 22px;
  line-height: 1;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}
.nav:hover { background: #fff; }
.nav--left { left: 12px; }
.nav--right { right: 12px; }
.thumbs {
  display: flex;
  gap: 8px;
  padding: 10px 12px;
  overflow-x: auto;
  background: #fff;
  border-top: 1px solid var(--border);
}
.thumb {
  flex-shrink: 0;
  width: 64px;
  height: 44px;
  border-radius: 6px;
  overflow: hidden;
  border: 1px solid var(--border);
  background: #f5f5f5;
  cursor: pointer;
  padding: 0;
}
.thumb img { width: 100%; height: 100%; object-fit: cover; display: block; }
.thumb--active { border-color: #0a0a0a; box-shadow: 0 0 0 2px #0a0a0a; }

/* footer */
.footer {
  margin-top: 20px;
  display: flex;
  gap: 8px;
  font-size: 12px;
  color: var(--text-secondary);
}
.footer a {
  color: #0a0a0a;
  text-decoration: underline;
  text-underline-offset: 3px;
}
.footer a:hover {
  color: #52525b;
}
</style>
