# Перенос SCRP на новый VPS — ранбук

Проверено на реальном переносе `vps-ams` (Arch, TimeWeb NL) → новый VPS
(Ubuntu 26.04, Koara, 2 CPU / 3.8 GiB / 59 GB). Все команды ниже выполнялись
и дали указанный результат.

## 0. Источники компонентов

| Компонент | Где берём | Размер |
|---|---|---|
| Код сервиса | `git clone https://github.com/mygoonet/SCR` (репозиторий **публичный**, креды для `git pull` на VPS не нужны) | 52 MB |
| `.env` (`CHROME_PATH`, `USER_DATA_DIR`, `CERT_USER`, `TG_TOKEN`, `TG_CHAT`) | **не в git** — копируем со старого VPS `/root/SCR/.env` | 5 строк |
| КриптоПро CSP (программа + лицензия + хранилища) | старый VPS: `/opt/cprocsp`, `/etc/opt/cprocsp`, `/var/opt/cprocsp` | 63 MB + 456 KB |
| HDIMAGE-контейнер с закрытым ключом | внутри `/var/opt/cprocsp/keys/root/<имя>.000` (у нас `myrcertr.000` = `\\.\HDIMAGE\my_cert_2027`) | ~3 KB |
| Chromium-Gost | локальная машина `/opt/chromium-gost` (или старый VPS) | 375 MB |
| kontur.plugin | `/opt/kontur.plugin` | 33 MB |
| Манифест native messaging | `/etc/chromium/native-messaging-hosts/kontur.plugin.json` | KB |
| Профиль браузера (сохранённая сессия Контура) | локальная `~/.config/chromium-gost-scrp` → на VPS `/root/chromium-profile` | 192 MB |

Скрипты и `docker-compose.yml` в репо не меняются: пути монтирования
(`/opt/chromium-gost`, `/opt/cprocsp`, `/var/opt/cprocsp`, `/etc/opt/cprocsp`,
`/opt/kontur.plugin`, `/etc/chromium`, `/root/chromium-profile`) остаются те же.

## 1. Сначала — доступность Kontur с нового IP (главный блокер)

На старом VPS сервис «молча не подписывал» именно поэтому: Kontur не принимал
соединения с иностранного IP, при этом остальные RU-сайты работали.

```bash
curl -sS -m 12 -o /dev/null -w 'logist.kontur.ru -> %{http_code} (%{time_total}s)\n' https://logist.kontur.ru/
timeout 8 bash -c 'exec 3<>/dev/tcp/46.17.200.233/443' && echo 'tcp 46.17.200.233:443 OK' || echo 'FAIL'
```

Ожидалось на старом (`201.34.147.129`, NL): `000` и `FAIL`.
На новом (`212.113.99.207`): `200 (0.21s)` и `OK`. `https://kontur.ru → 403` —
это WAF маркетингового сайта, для работы не важен. **Если 000/FAIL — переносить
смысла нет**, нужен российский выход для контейнера.

## 2. Доступ на новый VPS: свои SSH-ключи

Ключ деплоя GitHub Actions и оба личных ключа добавляются в
`/root/.ssh/authorized_keys` (права 700 на каталог, 600 на файл):

```bash
cat ~/.ssh/deploy_scr.pub ~/.ssh/id_ed25519_nopass.pub ~/.ssh/id_rsa.pub | \
  ssh root@NEW 'install -d -m 700 /root/.ssh; umask 077; sort -u -k2 >> /root/.ssh/authorized_keys'
ssh-keygen -lf ~/.ssh/deploy_scr.pub          # сверить отпечаток с authorized_keys
```

`deploy_scr` — OpenSSH-формат, без passphrase → его и класть в секрет
`VPS_SSH_KEY`. Ключи с passphrase из CI не годятся.

## 3. Базовые пакеты

В репозиториях Ubuntu `docker-compose-plugin` нет (на 26.04 проверено) —
ставим docker.io и бинарник compose v2 как cli-плагин:

```bash
export DEBIAN_FRONTEND=noninteractive
apt-get update && apt-get install -y git docker.io apparmor rsync
systemctl enable --now docker
mkdir -p /usr/lib/docker/cli-plugins
ver=$(curl -sS https://api.github.com/repos/docker/compose/releases/latest | sed -n 's/.*"tag_name": "\(v[0-9.]*\)".*/\1/p' | head -1)
curl -sSL -o /usr/lib/docker/cli-plugins/docker-compose \
  "https://github.com/docker/compose/releases/download/${ver}/docker-compose-linux-x86_64"
chmod +x /usr/lib/docker/cli-plugins/docker-compose
docker --version && docker compose version
```

Проверено: `Docker version 29.1.3`, `Docker Compose version v5.6.1`, `git 2.53.0`.

## 4. Код и `.env`

```bash
cd /root && git clone https://github.com/mygoonet/SCR && cd SCR && git log --oneline -1

# .env переносим трубой, не печатая значения токенов
ssh OLD 'cat /root/SCR/.env' | ssh NEW 'install -d -m 700 /root/SCR; umask 077; cat > /root/SCR/.env'
grep -vE 'TG_TOKEN|TG_CHAT' /root/SCR/.env   # проверить не-секретные значения
```

## 5. КриптоПро CSP

Официальный `.deb` не понадобился: бинарники из рабочего устаовки требуют только
`GLIBC_2.14` (проверка: `objdump -T /opt/cprocsp/bin/amd64/csptest | grep -oE 'GLIBC_[0-9.]+' | sort -Vu | tail -1`),
так что дерево переносится целиком даже с Arch-хоста на Ubuntu:

```bash
ssh OLD 'tar cz -C / opt/cprocsp etc/opt/cprocsp var/opt/cprocsp' | ssh NEW 'tar xz -C /'
```

Переносить надо **все три** каталога: `/etc/opt/cprocsp/license.ini` —
активация (на новом хосте заработала без повторной регистрации),
`/var/opt/cprocsp` — контейнеры HDIMAGE (`keys/root/…`), личные хранилища
сертификатов (`users/…/stores`) и `tmpcerts`.

Проверки на новом хосте:

```bash
export LD_LIBRARY_PATH=/opt/cprocsp/lib/amd64
ldd /opt/cprocsp/bin/amd64/csptest | grep 'not found'      # пусто = библиотеки на месте
/opt/cprocsp/bin/amd64/csptest -keyset -enum_cont -fqcn -verifyc
#   CSP (Type:80) v5.0.10014 ... Release Ver:5.0.13700 ... AcquireContext: OK
#   \\.\HDIMAGE\my_cert_2027
# реальный тест закрытого ключа (подпись создаётся без запроса PIN):
cd /root && echo test > kk.txt
/opt/cprocsp/bin/amd64/cryptcp -signf -key '\\.\HDIMAGE\my_cert_2027' kk.txt
#   Signed message is created. [ErrorCode: 0x00000000]  → ключ рабочий
rm -f /root/kk.txt /root/kk.txt.sgn
```

Если контейнер был с PIN —cryptcp спросит его; для headless-работы контейнер
должен быть без PIN (в текущей установке PIN не запрашивается).

## 6. Chromium-Gost, kontur.plugin, профиль

```bash
# браузер + плагин + манифест native messaging (пути в манифесте абсолютные — совпадают)
tar cz -C / opt/chromium-gost opt/kontur.plugin etc/chromium | ssh NEW 'tar xz -C /'

# профиль сессии: кэжи не нужны, важный мусор исключаем; --exclude ДО пути!
tar --exclude='*/Cache/*' --exclude='*/Code Cache/*' --exclude='*/Crashpad/*' \
    --exclude='*/GPUCache/*'  --exclude='*/Service Worker/*' \
    -cz -C ~/.config chromium-gost-scrp | \
  ssh NEW 'rm -rf /root/chromium-profile; mkdir -p /root/chromium-profile;
           tar xz -C /root/chromium-profile --strip-components=1;
           chown -R root:root /root/chromium-profile'
# контроль: число файлов совпало, Default/Cookies на месте
```

Профиль с валидной сессией = старт без полного логина (`initSession: already
logged in`). Без профиля сервис залогинится сам по сертификату, но дольше и с
риском застрять на `auth.kontur.ru` (см. `.pi/skills/scr-healthcheck`).

## 7. Сборка и ПРОВЕРКА БЕЗ ПОДПИСАНИЯ

```bash
cd /root/SCR && docker compose build            # scr-scr:latest
```

Первый запуск `up -d` **сразу вызовет `checkAndSign()` и подпишет накладные**.
Поэтому сначала гоняем чекер в том же контейнерном окружении (он останавливается
на модалке выбора сертификата и жмёт ESC):

```bash
mkdir -p out
docker run --rm -v /root/SCR:/src -w /src golang:1.26-bookworm go build -o /src/out/scrpcheck ./cmd/scrpcheck
docker compose run --rm --entrypoint /out/scrpcheck -v /root/SCR/out:/out:ro scr
```

Хороший исход (проверено на новом VPS):

```
initSession: already logged in → carrier OK → waybill rows = 5
Login: true   OnCarrier: true   Notes count: 5
[OK] … certificate list=1   certificate=Сичкарук Евгений Александрович=1
Sign flow: true
RESULT: OK — разметка не поменялась, путь подписания жив
```

`certificate=… count=1` означает, что цепочка Chromium-Gost → kontur.plugin →
CryptoPro → сертификат собралась в контейнере.

## 8. Запуск

```bash
cd /root/SCR && docker compose up -d
docker logs -f scr-scr-1          # «MonitorAPI запущен. Тикер: 6m0s»
curl -s localhost:2000/api/status # статус сессии/последнего обхода
curl -s localhost:2000/api/notes  # список накладных
```

Признаки неисправности в логе:

```
MonitorAPI: init session (попытка N, повтор через 5m0s): ... net::ERR_CONNECTION_TIMED_OUT
   → Kontur недоступен с IP хоста (см. шаг 1) или нет выхода в интернет
Browser: предпроверка chromium не прошла: ...
   → сломан/отсутствует CHROME_PATH, не смонтирован /opt/chromium-gost
certificate list=0 в чекере
   → не поднялся kontur.plugin / не видно сертификата в хранилище root
```

## 9. GitHub Actions

`deploy.yml` по пушу в `master` выполняет на `secrets.VPS_HOST`:
`cd ~/SCR && git pull && docker compose stop scr && docker compose build && docker compose up -d`.

Поэтому **до первого пуша** перевести секреты репозитория на новый хост:
`VPS_HOST=…`, `VPS_PORT=22`, `VPS_USER=root`, `VPS_SSH_KEY` = приват `deploy_scr`.
Иначе пуш поднимет контейнер на СТАРОМ хосте, несмотря на `docker stop`
(политика `unless-stopped` не мешает явному `compose up -d`).

## 10. Старый VPS после переезда

```bash
docker stop scr-scr-1                     # временно (переживает перезагрузку хоста)
docker update --restart=no scr-scr-1      # чтобы точно не поднимался
cd /root/SCR && docker compose down       # убрать контейнер и сеть проекта
mv /root/SCR /root/SCR.disabled           # если надо исключить случайный пересборочный деплой
```

Крипто-компоненты на старом хосте (`/opt/cprocsp`, `/var/opt/cprocsp`) —
копия закрытых ключей; при отказе от старого VPS их следует удалить, а ключ
на всякий случай перевыпустить.

## 11. Собранные грабли

1. **«Не подписывает» без ошибок в логе** — `MonitorAPI` при первой же ошибке
   `initSession` делал `return`: контейнер оставался `Up` с одним веб-сервером.
   Исправлено в `SCRP/monitor_api.go` (повтор с backoff 1m→2m→5m, telegram не
   чаще раза в 30 мин).
2. **Паника `close of closed channel`** в `chromedp v0.16.0`, если процесс
   chromium завершается немедленно; `recover()` не помогает (чужая горутина).
   Исправлено предпроверкой бинарника и пересозданием аллокатора
   (`SCRP/browser.go`: `CheckChromeBinary`, `allocate`).
3. **Гео-блокировка Kontur** по IP хостинга — проверяется до переноса (шаг 1).
4. `--exclude` у `tar` positional: ставить **до** пути, иначе молча не работает.
5. `docker compose run` не публикует порты — для проверки это плюс.
6. Профиль из-под пользователя (`uid 1000`) после переноса требует
   `chown -R root:root`, если контейнер работает от root.
7. `docker-compose-plugin` в Ubuntu нет — только cli-плагин из релизов compose
   или репозиторий docker.com.
