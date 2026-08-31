<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'

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
    (n.status && n.status.toLowerCase().includes(q))
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
  <div>
    <!-- header full-width -->
    <header class="border-b border-gray-200 pt-7 pb-5 bg-white sticky top-0 z-10">
      <div style="width:100%;max-width:960px;margin-left:auto;margin-right:auto" class="grid grid-cols-3 items-center gap-x-8 px-4 sm:px-6 lg:px-8">
        <div class="flex flex-col gap-0.5">
          <h1 class="text-lg sm:text-xl font-semibold tracking-tight text-black leading-tight">Накладные</h1>
          <p class="text-xs text-gray-400 leading-snug">Контур Логистика · автоматическое подписание</p>
        </div>
        <div class="self-center text-center">
          <div class="inline-block border border-gray-200 rounded-lg px-3 py-2 bg-white">
            <span class="block text-xl font-semibold leading-none text-black">{{ notes.length }}</span>
            <span class="text-[11px] uppercase tracking-widest text-gray-400">всего</span>
          </div>
        </div>
        <div class="text-right">
          <button @click="syncNow" class="inline-flex items-center gap-1.5 text-xs text-gray-400 whitespace-nowrap min-w-[110px] hover:text-black cursor-pointer bg-transparent border-none p-0" :class="{ '!text-black': isUpdating }">
            <span class="w-2 h-2 rounded-full flex-shrink-0 relative" :class="countdownPulse ? 'bg-black dot--pulse' : 'bg-gray-400'"></span>
            <span :class="{ 'animate-countdown-pop': countdownPulse }">{{ countdown }}с</span>
          </button>
        </div>
      </div>
    </header>

    <!-- content centered -->
    <div style="width:100%;max-width:960px;margin-left:auto;margin-right:auto" class="px-4 sm:px-6 lg:px-8 pb-10">
    <!-- status bar - по центру как хедер, full width внутри контейнера -->
    <div v-if="status" class="mt-4 relative grid grid-cols-[1fr_auto] items-stretch w-full bg-gray-50 border border-gray-200 rounded-lg overflow-hidden min-w-0">
      <div class="absolute inset-0 bg-gray-200 transition-all duration-1000 pointer-events-none" :style="{ width: tickerProgress + '%' }"></div>
      <div class="relative flex items-baseline gap-1.5 sm:gap-2 px-3 py-2 min-w-0">
        <span class="text-[10px] sm:text-[11px] uppercase tracking-widest text-gray-400 font-medium whitespace-nowrap shrink-0 leading-none">last tic</span>
        <span class="font-bold text-[13px] sm:text-[15px] text-black whitespace-nowrap tabular-nums leading-none">{{ splitDT(status.lastFetchTime).t || '—' }}</span>
      </div>
      <div v-if="secondsSinceFetch != null" class="relative flex items-center justify-center px-3 sm:px-5 min-w-[66px] sm:min-w-[90px] shrink-0">
        <span class="font-bold text-sm sm:text-base text-black whitespace-nowrap tabular-nums leading-none">{{ formatSec(secondsSinceFetch) }}</span>
      </div>
      <div v-if="status.lastFetchError" class="relative col-span-2 border-t border-red-200 bg-red-50 px-3 py-1.5 flex gap-2 items-baseline flex-wrap">
        <span class="text-[10px] uppercase tracking-widest font-medium shrink-0" style="color:#991b1b">Ошибка</span>
        <span class="text-xs sm:text-sm break-all" style="color:#991b1b">{{ status.lastFetchError }}</span>
      </div>
    </div>

    <!-- errors -->
    <div v-if="status?.signingFailures?.length" class="mt-3 border border-gray-200 border-l-[3px] border-l-black bg-gray-50 rounded-lg px-3.5 py-3">
      <div class="text-xs font-semibold tracking-wide text-black mb-1.5">Критические ошибки · {{ status.signingFailures.length }}</div>
      <ul class="ml-4 text-sm text-gray-600 space-y-0.5">
        <li v-for="(e,i) in status.signingFailures" :key="i">{{ e }}</li>
      </ul>
    </div>

    <div v-if="error" class="mt-3 border-l-[3px] border-l-red-500 bg-red-50 text-red-800 rounded-lg px-3.5 py-3">
      Ошибка загрузки: {{ error }}
    </div>

    <!-- toolbar -->
    <div class="mt-4 flex items-center gap-3">
      <input v-model="query" placeholder="Номер, отправитель, получатель, водитель, грузовик, статус…" class="flex-1 max-w-[420px] h-9 px-3 border border-gray-200 rounded-lg bg-white text-sm text-black outline-none placeholder:text-gray-400 focus:border-black focus:ring-1 focus:ring-black/5" />
      <span class="text-xs text-gray-400">{{ filtered.length }} из {{ notes.length }}</span>
    </div>

    <!-- empty -->
    <div v-if="!loading && !filtered.length" class="mt-6 border border-dashed border-gray-200 rounded-xl py-10 px-6 text-center bg-gray-50">
      <div class="text-2xl text-gray-400">—</div>
      <div class="mt-2 font-semibold text-black">Нет данных</div>
      <div class="mt-1 text-sm text-gray-500">Накладные появятся после следующего тикера</div>
    </div>

    <!-- table: адаптив без скрытия полей -->
    <div v-else class="mt-4 border border-gray-200 rounded-xl overflow-hidden sm:overflow-x-auto bg-white">
      <table class="w-full table-fixed sm:table-auto border-collapse text-sm">
        <colgroup>
          <col class="w-[28%] sm:w-auto" />
          <col class="w-[40%] sm:w-auto" />
          <col class="w-[20%] sm:w-[110px]" />
          <col class="w-[12%] sm:w-[64px]" />
        </colgroup>
        <thead>
          <tr>
            <th class="text-left text-[10px] sm:text-[11px] uppercase tracking-widest text-gray-400 font-semibold bg-gray-50 border-b border-gray-200 px-1.5 sm:px-3 py-2 sm:py-2.5 leading-tight whitespace-normal sm:whitespace-nowrap">Номер / Дата</th>
            <th class="text-left text-[10px] sm:text-[11px] uppercase tracking-widest text-gray-400 font-semibold bg-gray-50 border-b border-gray-200 px-1.5 sm:px-3 py-2 sm:py-2.5 leading-tight whitespace-normal sm:whitespace-nowrap">Маршрут</th>
            <th class="text-center text-[10px] sm:text-[11px] uppercase tracking-widest text-gray-400 font-semibold bg-gray-50 border-b border-gray-200 px-1.5 sm:px-3 py-2 sm:py-2.5 leading-tight whitespace-normal sm:whitespace-nowrap">Статус</th>
            <th class="text-center text-[10px] sm:text-[11px] uppercase tracking-widest text-gray-400 font-semibold bg-gray-50 border-b border-gray-200 px-1.5 sm:px-3 py-2 sm:py-2.5 leading-tight">Pic</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="n in filtered" :key="n.number" class="border-b border-gray-100 last:border-none hover:bg-gray-50">
            <!-- номер / дата -->
            <td class="px-1.5 sm:px-3 py-2 sm:py-2.5 align-top">
              <div class="flex flex-col gap-0.5 min-w-0">
                <span class="font-mono font-semibold text-[13px] sm:text-sm leading-none truncate">{{ n.number }}</span>
                <span class="text-[11px] sm:text-xs text-gray-400 leading-none">{{ n.date || '—' }}</span>
                <span class="text-[10px] sm:text-[11px] text-gray-400 flex gap-1 items-baseline leading-none"><span class="uppercase tracking-wide font-medium shrink-0">созд.</span> <span class="truncate">{{ splitDT(n.createdAt).t || '—' }}</span></span>
                <span class="text-[10px] sm:text-[11px] text-gray-400 flex gap-1 items-baseline leading-none"><span class="uppercase tracking-wide font-medium shrink-0">подп.</span> <span class="truncate">{{ splitDT(signedAt(n)).t || '—' }}</span></span>
              </div>
            </td>
            <!-- маршрут -->
            <td class="px-1.5 sm:px-3 py-2 sm:py-2.5 align-top">
              <div class="flex flex-col gap-0.5 leading-snug min-w-0">
                <div class="text-xs sm:text-sm text-black break-words [overflow-wrap:anywhere] leading-tight"><span class="text-[10px] sm:text-[11px] uppercase tracking-wide text-gray-400 font-medium">Отпр.</span> {{ n.consignor || '—' }}</div>
                <div class="text-xs sm:text-sm text-black break-words [overflow-wrap:anywhere] leading-tight"><span class="text-[10px] sm:text-[11px] uppercase tracking-wide text-gray-400 font-medium">Получ.</span> {{ n.consignee || '—' }}</div>
                <div v-if="n.driver || n.truck" class="text-[11px] sm:text-xs text-gray-400 leading-tight truncate">
                  <span v-if="n.driver">{{ n.driver }}</span>
                  <span v-if="n.driver && n.truck"> · </span>
                  <span v-if="n.truck" class="font-mono">{{ n.truck }}</span>
                </div>
                <div class="text-[11px] sm:text-xs text-gray-400 leading-tight break-words [overflow-wrap:anywhere] line-clamp-2">{{ n.consignorAddress || '—' }} → {{ n.consigneeAddress || '—' }}</div>
                <div v-if="n.receptionAddress" class="text-[11px] sm:text-xs text-gray-400 leading-tight break-words [overflow-wrap:anywhere]"><span class="text-[10px] sm:text-[11px] uppercase tracking-wide font-medium">Приём:</span> <strong class="font-semibold text-gray-600 break-words">{{ n.receptionAddress }}</strong></div>
                <div v-if="n.deliveryAddress" class="text-[11px] sm:text-xs text-gray-400 leading-tight break-words [overflow-wrap:anywhere]"><span class="text-[10px] sm:text-[11px] uppercase tracking-wide font-medium">Доставка:</span> <strong class="font-semibold text-gray-600 break-words">{{ n.deliveryAddress }}</strong></div>
              </div>
            </td>
            <!-- статус -->
            <td class="px-1.5 sm:px-3 py-2 sm:py-2.5 align-middle text-center">
              <template v-if="n.status">
                <span v-if="n.status === 'signed'" class="inline-flex items-center justify-center h-[18px] sm:h-5 px-1.5 sm:px-2 rounded-full text-[10px] sm:text-[11px] font-semibold bg-black text-white border border-black whitespace-nowrap leading-none">Sign</span>
                <span v-else-if="n.status === 'failed'" class="inline-flex items-center justify-center h-[18px] sm:h-5 px-1.5 sm:px-2 rounded-full text-[10px] sm:text-[11px] font-semibold bg-white text-red-700 border border-red-200 whitespace-nowrap leading-none">ошибка</span>
                <span v-else class="inline-flex items-center justify-center h-[18px] sm:h-5 px-1.5 sm:px-2 rounded-full text-[10px] sm:text-[11px] font-semibold bg-gray-50 text-gray-600 border border-gray-200 whitespace-nowrap leading-none">{{ n.status }}</span>
              </template>
              <span v-if="n.error" class="block text-[11px] sm:text-xs text-red-700 leading-tight break-words [overflow-wrap:anywhere] mt-1 line-clamp-3">{{ n.error }}</span>
              <span v-if="!n.status && !n.error" class="text-gray-400 text-xs">—</span>
            </td>
            <!-- скриншоты -->
            <td class="px-1 sm:px-3 py-2 sm:py-2.5 align-middle text-center">
              <template v-if="n.shots?.length">
                <button class="mx-auto block w-7 h-5 sm:w-9 sm:h-6 border border-gray-200 rounded overflow-hidden bg-gray-50 cursor-pointer p-0" @click="openCarousel(n,0)" :title="n.shots[0]">
                  <img :src="`/screenshots/${n.number}/${n.shots[0]}`" loading="lazy" class="w-full h-full object-cover" />
                </button>
                <span v-if="n.shots.length > 1" class="block text-[10px] sm:text-xs text-gray-400 leading-none mt-0.5">+{{ n.shots.length - 1 }}</span>
              </template>
              <span v-else class="text-gray-400 text-xs">—</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- ticker notes -->
    <div v-if="status?.lastNotes?.length" class="mt-4 border border-gray-200 rounded-xl overflow-hidden bg-white">
      <div class="flex justify-between items-baseline px-3.5 py-2.5 border-b border-gray-200 bg-gray-50">
        <h3 class="text-sm font-semibold text-black">Последний тикер</h3>
        <span class="text-gray-400 text-xs">{{ status.lastNotesCount }} накладных</span>
      </div>
      <div class="px-3.5 py-2">
        <div v-for="n in status.lastNotes" :key="n.number" class="flex gap-3 py-1.5 border-b border-gray-100 last:border-none text-[12.5px] flex-wrap">
          <span class="font-semibold">{{ n.number }}</span>
          <span class="text-gray-400">от {{ n.date }}</span>
          <span>{{ n.consignor }} → {{ n.consignee }}</span>
          <span class="text-gray-400">{{ n.carrier }}</span>
        </div>
      </div>
    </div>

    <!-- carousel modal -->
    <Teleport to="body">
      <div v-if="showCarousel" class="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 p-6" @click.self="closeCarousel">
        <div class="bg-white rounded-xl overflow-hidden w-[min(1100px,96vw)] max-h-[90vh] flex flex-col border border-gray-200">
          <div class="flex items-center gap-3 px-3.5 py-2.5 border-b border-gray-200 bg-gray-50">
            <span class="font-mono font-semibold text-sm">{{ cNumber }} · {{ cIndex + 1 }} / {{ cShots.length }}</span>
            <span class="font-mono text-xs text-gray-400 truncate max-w-[40vw]">{{ cShots[cIndex] }}</span>
            <button class="ml-auto w-7 h-7 rounded-lg border border-gray-200 bg-white cursor-pointer text-sm flex items-center justify-center hover:border-black" @click="closeCarousel">✕</button>
          </div>
          <div class="relative bg-black flex items-center justify-center min-h-[320px] max-h-[62vh]">
            <button class="absolute left-3 top-1/2 -translate-y-1/2 w-9 h-9 rounded-full border border-white/30 bg-white/90 text-black text-xl cursor-pointer flex items-center justify-center" @click="prev" aria-label="prev">‹</button>
            <img :src="shotUrl(cShots[cIndex])" class="max-w-full max-h-[62vh] object-contain" />
            <button class="absolute right-3 top-1/2 -translate-y-1/2 w-9 h-9 rounded-full border border-white/30 bg-white/90 text-black text-xl cursor-pointer flex items-center justify-center" @click="next" aria-label="next">›</button>
          </div>
          <div class="flex gap-2 px-3 py-2.5 overflow-x-auto bg-white border-t border-gray-200">
            <button v-for="(s,i) in cShots" :key="s" class="flex-shrink-0 w-16 h-11 rounded-lg overflow-hidden border border-gray-200 bg-gray-50 cursor-pointer p-0" :class="{ 'border-black ring-2 ring-black/10': i === cIndex }" @click="cIndex = i">
              <img :src="shotUrl(s)" loading="lazy" class="w-full h-full object-cover" />
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <footer class="mt-5 flex gap-2 text-xs text-gray-500">
      <a href="/" class="text-black underline underline-offset-1 hover:text-gray-600">Legacy HTML</a>
      <span>·</span>
      <span>Обновление данных каждые 5 секунд</span>
    </footer>
    </div>
  </div>
</template>

<style scoped>
@keyframes dot-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.35; }
}
.animate-dot-blink {
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
@keyframes dot-ping {
  0% { transform: scale(1); opacity: 0.45; }
  100% { transform: scale(2.4); opacity: 0; }
}
@keyframes countdown-pop {
  0%   { transform: scale(1); }
  40%  { transform: scale(1.5); color: #0a0a0a; }
  100% { transform: scale(1); }
}
.animate-countdown-pop {
  animation: countdown-pop 0.5s cubic-bezier(0.175, 0.885, 0.32, 1.275);
}
</style>
