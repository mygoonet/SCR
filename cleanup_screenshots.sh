#!/usr/bin/env bash
set -euo pipefail

SCREENSHOTS_DIR="${1:-/home/visa/SCRP/screenshots}"
KEEP=15

# Убедимся, что каталог существует
if [[ ! -d "$SCREENSHOTS_DIR" ]]; then
  echo "Каталог $SCREENSHOTS_DIR не найден"
  exit 1
fi

# Список подкаталогов, отсортированный по времени модификации (новые сверху)
mapfile -t dirs < <(find "$SCREENSHOTS_DIR" -maxdepth 1 -mindepth 1 -type d -printf '%T@ %p\n' | sort -nr | cut -d' ' -f2-)

total=${#dirs[@]}
if (( total <= KEEP )); then
  echo "Папок $total <= $KEEP, очистка не нужна"
  exit 0
fi

# Удаляем всё, кроме первых KEEP
to_delete=("${dirs[@]:KEEP}")
echo "Удаляю ${#to_delete[@]} старых папок из $SCREENSHOTS_DIR"
for d in "${to_delete[@]}"; do
  echo "  rm -rf \"$d\""
  rm -rf "$d"
done

echo "Осталось папок: $(find "$SCREENSHOTS_DIR" -maxdepth 1 -mindepth 1 -type d | wc -l)"
