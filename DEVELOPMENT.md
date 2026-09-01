# 🚀 SCR — Локальная разработка

## Архитектура

Проект разделён на **3 независимых сервиса**:

```
┌─────────────┐     CORS/proxy     ┌─────────────┐
│  Backend Go │ ◄────────────────► │ Frontend Vue│
│  :2000      │                    │  :5173      │
│             │                    │             │
│ • API       │                    │ Vue 3 SPA   │
│ • Scraper   │                    │ Pinia store │
│ • Browser   │                    │             │
│ • Telegram  │                    │             │
└──────┬──────┘                    └─────────────┘
       │
       │ скриншоты + логи
       ▼
┌─────────────┐
│  DevTools   │
│  :9090      │ ← дашборд ошибок
└─────────────┘
```

## Запуск

### Вариант 1: Все сервисы сразу (рекомендуется)

```bash
./scripts/run-all.sh
```

Откроются три сервиса:
- **Backend Go**: http://localhost:2000
- **Frontend Vue**: http://localhost:5173
- **DevTools**: http://localhost:9090

### Вариант 2: По отдельности

```bash
# Backend Go
./scripts/run-backend.sh

# Frontend Vue
./scripts/run-frontend.sh

# DevTools
./scripts/run-devtools.sh
```

## Сервисы

### 1️⃣ Backend Go (`main.go`)
- **Порт**: 2000
- **API**:
  - `GET /api/notes` — список накладных
  - `GET /api/status` — статус тикера и ошибки
- **Vue SPA**: `GET /vue/`
- **Скриншоты**: `GET /screenshots/{номер}/`

### 2️⃣ Frontend Vue (`frontend/`)
- **Порт**: 5173 (Vite dev server)
- **Proxy**: `/api` → `localhost:2000`
- **Стек**: Vue 3 + Vite + Pinia + Vue Router

### 3️⃣ DevTools (`devtools/main.go`)
- **Порт**: 9090
- **Dashboard**: http://localhost:9090
- **API**:
  - `GET /api/errors` — список ошибок
  - `POST /api/clear` — очистить ошибки
- **Мониторит**: директорию `screenshots/` на наличие failed-подписей

## Переменные окружения

| Переменная | Описание | По умолчанию |
|------------|----------|--------------|
| `WEB_ADDR` | Адрес HTTP сервера | `:2000` |
| `SCREENSHOT_DIR` | Директория скриншотов | `./screenshots` |
| `CHROME_PATH` | Путь к Chromium Gost | — |
| `CERT_USER` | Сертификат пользователя | — |
| `TG_TOKEN` | Telegram bot token | встроенный |
| `TG_CHAT` | Telegram chat ID | встроенный |

## Структура

```
SCR/
├── main.go              # Backend Go (точка входа)
├── SCRP/                # Backend пакеты
│   ├── web.go           # HTTP сервер + API
│   ├── scraper.go       # Парсинг
│   ├── browser.go       # Управление браузером
│   └── ...
├── frontend/            # Frontend Vue
│   ├── src/
│   │   ├── components/
│   │   ├── views/
│   │   └── stores/
│   └── vite.config.js   # Proxy к бэкенду
├── devtools/            # DevTools
│   └── main.go          # Дашборд ошибок
├── scripts/             # Скрипты запуска
│   ├── run-all.sh       # Все сервисы
│   ├── run-backend.sh   # Только backend
│   ├── run-frontend.sh  # Только frontend
│   └── run-devtools.sh  # Только devtools
└── screenshots/         # Скриншоты и note.json
```

## Отладка

1. **Открыть DevTools**: http://localhost:9090
2. **Смотреть ошибки** в реальном времени
3. **Фильтровать** по типу: fetch, sign, browser, api
4. **Очистить** ошибки кнопкой

## Примечания

- Docker не используется для локальной разработки
- Каждый сервис работает независимо
- Frontend проксирует `/api` и `/screenshots` к бэкенду
- DevTools мониторит директорию скриншотов на наличие ошибок
