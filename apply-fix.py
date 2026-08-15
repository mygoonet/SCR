#!/usr/bin/env python3
"""Применяет замены data-tid-селекторов в SCRP/SCRP/*.go.

Формат fix-файла (создаётся навыком scrp-healthcheck):
    <старый_селектор><TAB><новый_селектор>
  - одна пара на строку;
  - пустые строки и строки, начинающиеся с '#', игнорируются;
  - замена буквальная по всем файлам пакета.

Перед заменой все .go файлы копируются в .backup-<timestamp>/.
После замены можно пересобрать и перепрогнать чекер:
    go build -o scrpcheck ./cmd/scrpcheck && ./scrpcheck

usage: apply-fix.py <fix-file>
"""
import os
import pathlib
import shutil
import sys
from datetime import datetime

REPO = pathlib.Path(__file__).resolve().parent
PACKAGE = pathlib.Path(os.environ.get("SCRP_PACKAGE_DIR", str(REPO / "SCRP")))
FILES = [
    "carrier.go", "sign.go", "check.go",
    "auth.go", "pages.go", "notelog.go", "scraper.go", "monitor_api.go",
]

def main() -> int:
    if len(sys.argv) != 2:
        print(__doc__)
        return 2
    fix_file = pathlib.Path(sys.argv[1])
    if not fix_file.exists():
        print(f"fix-файл не найден: {fix_file}")
        return 1

    repls: list[tuple[str, str]] = []
    for line in fix_file.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "\t" not in line:
            continue
        old, new = line.split("\t", 1)
        old, new = old.strip(), new.strip()
        if old and old != new:
            repls.append((old, new))

    if not repls:
        print("В fix-файле нет пар 'старый<TAB>новый' для замены.")
        return 1

    ts = datetime.now().strftime("%Y%m%d-%H%M%S")
    backup = REPO / f".backup-{ts}"
    backup.mkdir(exist_ok=True)

    total = 0
    for name in FILES:
        f = PACKAGE / name
        if not f.exists():
            continue
        text = f.read_text(encoding="utf-8")
        orig = text
        for old, new in repls:
            if old in text:
                print(f"  {name}: замена {old!r} -> {new!r}")
                text = text.replace(old, new)
        if text != orig:
            shutil.copy(f, backup / name)
            f.write_text(text, encoding="utf-8")
            total += 1

    print(f"\nОбновлено файлов: {total}")
    print(f"Бэкап оригиналов: {backup}")
    if total == 0:
        print("ВНИМАНИЕ: ни один селектор не найден в исходниках — проверь fix-файл.")
        return 1
    print("\nДалее:")
    print(f"  cd {REPO} && go build -o scrpcheck ./cmd/scrpcheck")
    print("  CHROME_PATH=/opt/chromium-gost/chromium-gost USER_DATA_DIR=$HOME/.config/chromium-gost-scrp CERT_USER='Сичкарук Евгений Александрович' ./scrpcheck")
    return 0

if __name__ == "__main__":
    sys.exit(main())