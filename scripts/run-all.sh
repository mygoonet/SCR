#!/bin/bash
# Запуск всех трёх сервисов параллельно

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "=========================================="
echo "🚀 Запуск всех сервисов SCR"
echo "=========================================="
echo ""

# Функция для запуска сервиса в фоне
run_service() {
    local name="$1"
    local script="$2"
    echo "▶  Запуск $name..."
    bash "$script" &
    PID=$!
    echo "   PID: $PID"
    SERVICES["$name"]="$PID"
}

declare -A SERVICES

# Запускаем все три сервиса
run_service "Backend Go"  "$SCRIPT_DIR/run-backend.sh"
run_service "Frontend Vue" "$SCRIPT_DIR/run-frontend.sh"
run_service "DevTools"    "$SCRIPT_DIR/run-devtools.sh"

echo ""
echo "=========================================="
echo "✅ Все сервисы запущены!"
echo "=========================================="
echo ""
echo "📊 Backend Go:     http://localhost:2000"
echo "🎨 Frontend Vue:   http://localhost:5173"
echo "🔧 DevTools:       http://localhost:9090"
echo ""
echo "Нажмите Ctrl+C для остановки всех сервисов"
echo ""

# Перехват Ctrl+C
trap 'echo ""; echo "⏹ Остановка всех сервисов..."; kill $(jobs -p) 2>/dev/null; exit 0' INT TERM

# Ждём все фоновые процессы
wait
