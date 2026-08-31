# SCRP Healthcheck — результат прогона `-fresh`

**Дата:** 2026-08-30 16:54  
**Флаг:** `-fresh` (сброс сессии, полный путь логина)  
**Профиль:** `/home/visa/.config/chromium-gost-scrp` → копия в `/tmp/scrp-fresh-*`

## Команда

```bash
cd /home/visa/SCRP && \
CHROME_PATH=/usr/bin/chromium-gost-stable \
USER_DATA_DIR=/home/visa/.config/chromium-gost-scrp \
CERT_USER="Сичкарук Евгений Александрович" \
./scrpcheck -fresh 2>&1
```

## Результат

```
purgeSessionFiles: удалено файлов сессии = 10
scrpcheck: -fresh использует КОПИЮ профиля /tmp/scrp-fresh-3929752238 (сессия сброшена)
CheckSite: принудительный сброс сессии перед проверкой
initSession: found Сертификат, логинюсь
Login: после входа url=https://logist.kontur.ru/callback?...
initSession: login OK
NavigateToCarrier: enter url=https://logist.kontur.ru/box-selection
NavigateToCarrier: clicked Перевозчик
initSession: carrier OK
ParseDeliveryRows: waybill rows = 6
```

### Сводка

| Поле | Значение |
|------|----------|
| Login | ✅ true |
| OnCarrier | ✅ true |
| Notes count | 6 |
| Sign flow | ✅ true |
| **RESULT** | **OK** |

### Селекторы

| Статус | Селектор | Count |
|--------|----------|-------|
| [OK] | TableRow | 6 |
| [OK] | WaybillNumber | 6 |
| [OK] | WaybillDate | 6 |
| [OK] | WaybillSenderCell | 6 |
| [OK] | WaybillRecipientCell | 6 |
| [OK] | CarrierName | 6 |
| [OK] | DriverName | 6 |
| [OK] | DriverPhone | 6 |
| [OK] | TruckInfo | 6 |
| [OK] | RowActions | 6 |
| [OK] | Popup__root | 1 |
| [OK] | SignWithoutDriverSignature | 1 |
| [OK] | SidePageFooter__root/Sign | 1 |
| [OK] | certificate list | 1 |
| [OK] | certificate=Сичкарук Евгений Александрович | 1 |

## Путь логина

1. Клик «Сертификат» → редирект на `auth.kontur.ru`
2. `ReactClickContains("Сичкарук Евгений Александрович")` → выбор имени
3. Callback на `logist.kontur.ru/callback?code=...`
4. Возврат на `logist.kontur.ru` → `login OK`
5. Клик «Перевозчик» → таблица накладных

## Статус

✅ Применено — чекер OK, разметка не менялась, полный путь логина работает.
