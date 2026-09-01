#!/bin/bash
# Запуск DevTools (порт 9090)

cd "$(dirname "$0")/.."

echo "🔧 Запуск DevTools..."
echo "   Порт: 9090"
echo "   Dashboard: http://localhost:9090"
echo "   API: http://localhost:9090/api/errors"
echo ""

export SCREENSHOT_DIR="./screenshots"

go run devtools/main.go
