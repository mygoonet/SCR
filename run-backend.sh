#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
set -a
source .env
set +a

echo "Запуск Go бэкенда..."
echo "CHROME_PATH=$CHROME_PATH"
echo "CERT_USER=$CERT_USER"
echo "WEB_ADDR=${WEB_ADDR:-:2000}"

exec go run main.go
