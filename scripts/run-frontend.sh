#!/bin/bash
# Запуск Frontend Vue (порт 5173)

cd "$(dirname "$0")/../frontend"

echo "🎨 Запуск Frontend Vue..."
echo "   Порт: 5173"
echo "   URL: http://localhost:5173"
echo "   Proxy к API: localhost:2000"
echo ""

npm run dev
