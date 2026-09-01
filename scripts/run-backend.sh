#!/bin/bash
# Запуск Backend Go (порт 2000)

cd "$(dirname "$0")/.."

echo "🚀 Запуск Backend Go..."
echo "   Порт: 2000"
echo "   API: http://localhost:2000/api/notes"
echo "   Vue: http://localhost:2000/vue/"
echo ""

export WEB_ADDR=":2000"
export SCREENSHOT_DIR="./screenshots"

go run main.go
