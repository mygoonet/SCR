package SCRP

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	neturl "net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
)

type apiDriver struct {
	Phone      string `json:"phone"`
	FirstName  string `json:"firstName"`
	LastName   string `json:"lastName"`
	MiddleName string `json:"middleName"`
}

type apiTransportation struct {
	ID            string    `json:"id"`
	WaybillNumber string    `json:"waybillNumber"`
	WaybillDate   string    `json:"waybillDate"`
	ConsignorName string    `json:"consignorName"`
	ConsigneeName string    `json:"consigneeName"`
	CarrierName   string    `json:"carrierName"`
	CurrentDriver apiDriver `json:"currentDriver"`
	TruckInfo     string    `json:"truckInfo"`
}

type apiTransportations struct {
	Items      []apiTransportation `json:"items"`
	TotalCount int                 `json:"totalCount"`
}

// pending хранит перехваченные (паузнутые в response-stage) запросы к
// /transportations: RequestID -> URL запроса.
// pausedAt хранит время паузы каждого перехваченного запроса — чтобы из
// карты можно было выбрать самый свежий, а не случайный.
var (
	pendingMu sync.Mutex
	pending   = map[fetch.RequestID]string{}
	pausedAt  = map[fetch.RequestID]time.Time{}
)

// lastFetchMu/lastFetchTime/lastNotes — состояние последнего успешного тикера для веб-интерфейса
var (
	lastFetchMu     sync.Mutex
	lastFetchTime   time.Time
	lastNotes       []DeliveryNote
	lastFetchError  string
	signingFailures []string
)

// startTransportationsCapture включает fetch-домен и перехватывает в
// response-stage запросы, чей URL заканчивается на /transportations.
// Запрос удерживается в паузе до обработки в основном потоке
// (fetch.GetResponseBody + fetch.ContinueResponse) — это самый надёжный
// способ получить тело ответа SPA вместе со свежими токенами.
func startTransportationsCapture(ctx context.Context) {
	pendingMu.Lock()
	pending = map[fetch.RequestID]string{}
	pausedAt = map[fetch.RequestID]time.Time{}
	pendingMu.Unlock()

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		return fetch.Enable().
			WithPatterns([]*fetch.RequestPattern{{
				URLPattern:   "*transportations*",
				RequestStage: fetch.RequestStageResponse,
			}}).Do(c)
	})); err != nil {
		log.Printf("startTransportationsCapture: fetch.Enable: %v", err)
	}

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		e, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		url := ""
		if e.Request != nil {
			url = e.Request.URL
		}
		// negotiate и прочие /transportations/* кроме точного /transportations - сразу пропускаем,
		// иначе пауза negotiate блокирует SPA и список никогда не придет (дедлок 30s).
		if u, err := neturl.Parse(url); err == nil {
			if strings.Contains(u.Path, "/negotiate") || !strings.HasSuffix(u.Path, "/transportations") {
				// не держим в паузе - отпускаем немедленно (в фоне, не блокируем ListenTarget)
				reqID := e.RequestID
				go func() {
					// фоном на коротком таймауте, ctx может быть уже в deadline
					bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					chromedp.Run(bg, chromedp.ActionFunc(func(c context.Context) error {
						return fetch.ContinueResponse(reqID).Do(c)
					}))
				}()
				return
			}
		}
		log.Printf("fetch-intercept: %s", url)

		pendingMu.Lock()
		pending[e.RequestID] = url
		pausedAt[e.RequestID] = time.Now()
		pendingMu.Unlock()
	})
}

// reloadNotesAPI перезагружает страницу, заставляя SPA сделать свежий
// запрос /transportations (актуальные токены), и даёт 5с на старт.
// Если текущая страница - waybill (/sign/waybill), Reload бесполезен (лог 10:11:42 location=.../waybill
// -> pending только negotiate) - делаем быстрый возврат на список без waitForTableRows
// (waitForTableRows под паузой fetch дедлочит 30с).
// SetCacheDisabled уже был сделан в initSession (SCRP/scraper.go:143) навсегда - здесь не нужен.
func reloadNotesAPI(ctx context.Context) error {
	start := time.Now()
	var loc string
	if err := chromedp.Run(ctx, chromedp.Location(&loc)); err != nil {
		log.Printf("reloadNotesAPI: Location error: %v (ctx.Err=%v)", err, ctx.Err())
		return fmt.Errorf("Location: %w", err)
	}
	log.Printf("reloadNotesAPI: start loc=%s ctx.Err=%v", loc, ctx.Err())
	if strings.Contains(loc, "/sign/") || strings.Contains(loc, "/waybill") {
		log.Printf("reloadNotesAPI: на waybill %s -> быстрый возврат на список", loc)
		// Продолжаем все паузы чтобы разблокировать SPA перед навигацией
		continuePaused(ctx)
		// Esc закрывает SidePage, если не помогло - прямой Navigate на /carrier
		chromedp.Run(ctx, chromedp.KeyEvent("\x1b"))
		chromedp.Run(ctx, chromedp.Sleep(800*time.Millisecond))
		if ctx.Err() != nil {
			log.Printf("reloadNotesAPI: ctx canceled after Esc, duration=%v", time.Since(start))
			return ctx.Err()
		}
		chromedp.Run(ctx, chromedp.Location(&loc))
		if strings.Contains(loc, "/sign") || strings.Contains(loc, "/waybill") {
			// Вырезаем хвост до /carrier - https://logist.kontur.ru/<box>/carrier
			if idx := strings.Index(loc, "/carrier"); idx != -1 {
				listURL := loc[:idx+len("/carrier")]
				log.Printf("reloadNotesAPI: прямой Navigate %s", listURL)
				chromedp.Run(ctx, chromedp.Navigate(listURL))
			} else {
				chromedp.Run(ctx, chromedp.NavigateBack())
			}
		}
		// Увеличен пауза 4s->7s: SPA медленная, 4s недостаточно, хром остается в рендере
		// и следующий SetCacheDisabled (если будет) виснет 60s.
		chromedp.Run(ctx, chromedp.Sleep(7*time.Second))
		log.Printf("reloadNotesAPI: waybill путь завершен, duration=%v", time.Since(start))
		return nil
	}
	// Обычный путь: Reload. SetCacheDisabled уже был в initSession (навсегда).
	// Перед Reload проверяем ctx - если уже deadline, нет смысла делать Reload.
	if ctx.Err() != nil {
		log.Printf("reloadNotesAPI: ctx canceled before Reload, duration=%v", time.Since(start))
		return ctx.Err()
	}
	if err := chromedp.Run(ctx, chromedp.Reload()); err != nil {
		return fmt.Errorf("Reload: %w (ctx.Err=%v, duration=%v)", err, ctx.Err(), time.Since(start))
	}
	if err := chromedp.Run(ctx, chromedp.Sleep(5*time.Second)); err != nil {
		return fmt.Errorf("Sleep after reload: %w", err)
	}
	log.Printf("reloadNotesAPI: обычный путь завершен, duration=%v", time.Since(start))
	return nil
}

// FetchNotesAPI получает список накладных, перехватив ответ SPA.
// timeoutSec — сколько секунд ждём появления/тела ответа.
// Общий таймаут = timeoutSec + 30с на каждую попытку (запас на reload+навигацию waybill+GetResponseBody),
// ретрай на свежем контексте иначе вторая попытка ловит deadline exceeded как в 10:11:52 и 11:19:37.
func FetchNotesAPI(ctx context.Context, timeoutSec int) ([]DeliveryNote, error) {
	withTimeout := func(parent context.Context) (context.Context, context.CancelFunc) {
		return context.WithTimeout(parent, time.Duration(timeoutSec+30)*time.Second)
	}

	outerCtx, outerCancel := withTimeout(ctx)
	notes, err := fetchNotesAPICtx(outerCtx, timeoutSec)
	outerCancel()
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "Invalid InterceptionId") {
			log.Printf("FetchNotesAPI: протухший перехват, повторяю (ретрай на свежем контексте)...")
			continuePaused(ctx)
			// пауза 2s: хром/SPA должны успокоиться после неудачной попытки
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
			retryCtx, retryCancel := withTimeout(ctx)
			defer retryCancel()
			notes, err = fetchNotesAPICtx(retryCtx, timeoutSec)
		} else if strings.Contains(errStr, "не перехвачен запрос") || strings.Contains(errStr, "Reload") || strings.Contains(errStr, "Location") {
			log.Printf("FetchNotesAPI: таймаут/Reload ошибка, повторяю (ретрай на свежем контексте)...")
			continuePaused(ctx)
			// увеличенная пауза 3s: хром мог зависнуть на SetCacheDisabled/Reload
			chromedp.Run(ctx, chromedp.Sleep(3*time.Second))
			retryCtx, retryCancel := withTimeout(ctx)
			defer retryCancel()
			notes, err = fetchNotesAPICtx(retryCtx, timeoutSec)
		} else if ctx.Err() != nil {
			log.Printf("FetchNotesAPI: общий таймаут (browser завис?): %v", err)
		} else if outerCtx.Err() != nil {
			log.Printf("FetchNotesAPI: таймаут попытки: %v", err)
		}
	}
	return notes, err
}

func fetchNotesAPICtx(ctx context.Context, timeoutSec int) ([]DeliveryNote, error) {
	// Сбрасываем перехваченные запросы перед перезагрузкой: навигация отменяет
	// висевшие паузные запросы (их InterceptionId становится невалидным), и они
	// не должны оставаться кандидатами на выборку.
	pendingMu.Lock()
	pending = map[fetch.RequestID]string{}
	pausedAt = map[fetch.RequestID]time.Time{}
	pendingMu.Unlock()

	if err := reloadNotesAPI(ctx); err != nil {
		return nil, fmt.Errorf("reload: %w", err)
	}

	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)

	// Ждём появления паузнутого запроса списка.
	var listID fetch.RequestID
	for listID == "" && time.Now().Before(deadline) {
		if ctx.Err() != nil {
			continuePaused(ctx)
			return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
		}
		pendingMu.Lock()
		var newest time.Time
		for id, url := range pending {
			if u, err := neturl.Parse(url); err == nil && strings.HasSuffix(u.Path, "/transportations") {
				if listID == "" {
					listID = id
					newest = pausedAt[id]
					continue
				}
				// Из карты берём самый свежий запрос: старые могли быть
				// отменены навигацией (Invalid InterceptionId).
				if pausedAt[id].After(newest) {
					listID = id
					newest = pausedAt[id]
				}
			}
		}
		pendingMu.Unlock()
		if listID == "" {
			if err := chromedp.Run(ctx, chromedp.Sleep(400*time.Millisecond)); err != nil {
				if ctx.Err() != nil {
					continuePaused(ctx)
					return nil, fmt.Errorf("context cancelled during wait: %w", ctx.Err())
				}
				log.Printf("FetchNotesAPI: sleep error: %v", err)
			}
		}
	}
	if listID == "" {
		pendingMu.Lock()
		urls := make([]string, 0, len(pending))
		for _, u := range pending {
			urls = append(urls, u)
		}
		pendingMu.Unlock()

		var loc string
		chromedp.Run(ctx, chromedp.Location(&loc))

		log.Printf("FetchNotesAPI: таймаут %ds, pending=%d, urls=%v, location=%s",
			timeoutSec, len(urls), urls, loc)

		continuePaused(ctx)
		return nil, fmt.Errorf("не перехвачен запрос /transportations за %ds (pending=%d, loc=%s)",
			timeoutSec, len(urls), loc)
	}

	// Читаем тело (запрос ещё в паузе — тело гарантированно доступно).
	var raw []byte
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		b, err := fetch.GetResponseBody(listID).Do(c)
		if err != nil {
			return err
		}
		raw = b
		return nil
	})); err != nil {
		continuePaused(ctx)
		return nil, fmt.Errorf("get response body: %w", err)
	}

	// Отпускаем все паузнутые (и сам список, и прочие совпадения).
	continuePaused(ctx)

	var resp apiTransportations
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	log.Printf("FetchNotesAPI: totalCount=%d, items=%d", resp.TotalCount, len(resp.Items))

	notes := make([]DeliveryNote, 0, len(resp.Items))
	for _, it := range resp.Items {
		if it.WaybillNumber == "" {
			continue
		}
		n := DeliveryNote{
			Number:      it.WaybillNumber,
			Date:        it.WaybillDate,
			Consignor:   it.ConsignorName,
			Consignee:   it.ConsigneeName,
			Carrier:     it.CarrierName,
			Driver:      driverFullName(it.CurrentDriver),
			DriverPhone: it.CurrentDriver.Phone,
			Truck:       it.TruckInfo,
		}
		notes = append(notes, n)
	}
	return notes, nil
}

// continuePaused отпускает все перехваченные запросы, чтобы страница не висела.
func continuePaused(ctx context.Context) {
	pendingMu.Lock()
	ids := make([]fetch.RequestID, 0, len(pending))
	for id := range pending {
		ids = append(ids, id)
	}
	pending = map[fetch.RequestID]string{}
	pausedAt = map[fetch.RequestID]time.Time{}
	pendingMu.Unlock()

	for _, id := range ids {
		chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			err := fetch.ContinueResponse(id).Do(c)
			if err != nil && strings.Contains(err.Error(), "Invalid InterceptionId") {
				// Запрос уже отменён навигацией — не страшно, остальные продолжаем.
				return nil
			}
			return err
		}))
	}
}

func driverFullName(d apiDriver) string {
	parts := []string{d.LastName, d.FirstName, d.MiddleName}
	name := ""
	for _, p := range parts {
		if p != "" {
			if name != "" {
				name += " "
			}
			name += p
		}
	}
	return name
}

// TestAPI — автономный прогон: логин + запрос списка через перехват fetch.
func TestAPI(browser *Browser, cfg Config) {
	session := browser.NewSession()
	defer session.Close()

	ctx := session.Ctx()

	if err := initSession(ctx, cfg); err != nil {
		log.Printf("TestAPI: init session: %v", err)
		return
	}

	startTransportationsCapture(ctx)

	notes, err := FetchNotesAPI(ctx, 20)
	if err != nil {
		log.Printf("TestAPI: fetch: %v", err)
		return
	}

	log.Printf("TestAPI: накладных %d:", len(notes))
	for _, n := range notes {
		log.Printf("  %s  от %s  %s → %s  %s", n.Number, n.Date, n.Consignor, n.Consignee, n.Carrier)
	}
}

// signAllAPI — подписание с проверкой исчезновения через API-перехват
// (FetchNotesAPI), а не через DOM. DOM-проверка (reloadNoCache +
// waitForTableRows + fetchNotes) здесь не работает: fetch-перехват держит
// запросы /transportations в паузе, страница не дорисовывает таблицу, и
// DOM-чтение даёт 0 строк, из-за чего цикл верификации висит.
// giveUp — накладные, которые не подписаны после 3 попыток: они скипаются
// до конца сессии (пока не уйдут из списка), чтобы не крутить цикл вечно.
func signAllAPI(ctx context.Context, certUser string, notes []DeliveryNote, tel *TelegramClient, giveUp map[string]bool) error {
	var firstErr error
	for _, n := range notes {
		if skipNumbers[n.Number] {
			log.Printf(">>> signAllAPI: пропускаю %s (тестовая)", n.Number)
			continue
		}
		if giveUp[n.Number] {
			log.Printf(">>> signAllAPI: пропускаю %s (не удалось подписать ранее)", n.Number)
			continue
		}

		nl := NewNoteLogger(n)
		setActiveLogger(nl)
		nl.Logf(">>> signAllAPI: начинаю подписание %s", n.Number)

		signed := false
		for attempt := 1; attempt <= 3; attempt++ {
			nl.Logf(">>> signAllAPI: подписываю %s (попытка %d)...", n.Number, attempt)
			if tel != nil {
				tel.Sendf(">>> signAllAPI: подписываю %s (попытка %d)...", n.Number, attempt)
			}

			signCtx, signCancel := context.WithTimeout(ctx, 3*time.Minute)
			err := SignDeliveryNote(signCtx, n.Number, certUser)
			signCancel()
			if err != nil {
				nl.Logf(">>> Ошибка подписания %s: %v", n.Number, err)
				if firstErr == nil {
					firstErr = err
				}
				closePopups(ctx)
				// Перезагружаем страницу между попытками — как signAll.
				// Используем FetchNotesAPI: bare reloadNotesAPI + waitForTableRows
				// не работают, т.к. fetch-перехват держит /transportations в паузе
				// и таблица не рендерится. FetchNotesAPI перехватывает запрос,
				// читает тело и отпускает его — после этого таблица рисуется.
				FetchNotesAPI(ctx, 20)
				waitForTableRows(ctx)
				continue
			}
			nl.Logf(">>> Накладная %s подписана, проверяю через API...", n.Number)
			if tel != nil {
				tel.Sendf(">>> Накладная %s подписана", n.Number)
			}
			closePopups(ctx)

			// Серверный список обновляется с лагом — опрашиваем через API
			// до 4 раз (по ~10с), прежде чем решить что накладная ещё в списке.
			signed = true
			for check := 0; check < 4; check++ {
				refreshed, err := FetchNotesAPI(ctx, 20)
				if err != nil {
					nl.Logf(">>> Ошибка обновления списка (API): %v", err)
					signed = false
					break
				}
				still := false
				for _, r := range refreshed {
					if r.Number == n.Number {
						still = true
						break
					}
				}
				if !still {
					signed = true
					break
				}
				nl.Logf(">>> Накладная %s ещё в списке (проверка %d/4), жду...", n.Number, check+1)
				chromedp.Run(ctx, chromedp.Sleep(10*time.Second))
				signed = false
			}
			if signed {
				break
			}
			nl.Logf(">>> Накладная %s не пропала после 4 проверок, повторяю подписание...", n.Number)
		}
		if !signed {
			giveUp[n.Number] = true
			nl.SetStatus("failed", "не подписана после 3 попыток")
			nl.Logf(">>> Накладная %s не подписана после 3 попыток — не смогу подписать, пропускаю до конца сессии", n.Number)
			if tel != nil {
				tel.Sendf(">>> Накладная %s не подписана после 3 попыток — не смогу подписать, пропускаю", n.Number)
			}
			lastFetchMu.Lock()
			signingFailures = append(signingFailures, fmt.Sprintf("%s — не подписана после 3 попыток", n.Number))
			lastFetchMu.Unlock()
		} else {
			nl.SetStatus("signed", "")
			firstErr = nil
			delete(giveUp, n.Number)
		}
		nl.Logf(">>> signAllAPI: закончена обработка %s", n.Number)
		nl.Close()
		setActiveLogger(nil)
	}
	return firstErr
}
func MonitorAPI(browser *Browser, cfg Config, tel *TelegramClient, cmdCh <-chan MonitorCmd) {

	var session *Session
	var ctx context.Context
	sessionStartedAt := time.Now()
	restartCount := 0

	// startSession создает новую сессию, инициализирует и включает fetch-перехват
	startSession := func() error {
		session = browser.NewSession()
		ctx = session.Ctx()
		if err := initSession(ctx, cfg); err != nil {
			session.Close()
			return fmt.Errorf("init session: %w", err)
		}
		startTransportationsCapture(ctx)
		sessionStartedAt = time.Now()
		return nil
	}

	// restartSession закрывает текущую сессию и создает новую (при зависании хрома/CDP)
	restartSession := func(reason string) error {
		log.Printf("MonitorAPI: перезапуск сессии (reason=%s, uptime=%v, restarts=%d)", reason, time.Since(sessionStartedAt), restartCount)
		if tel != nil {
			tel.Sendf("🔄 MonitorAPI: перезапуск сессии (reason=%s, uptime=%v, restarts=%d)", reason, time.Since(sessionStartedAt).Round(time.Minute), restartCount)
		}
		if session != nil {
			session.Close()
		}
		if err := startSession(); err != nil {
			return err
		}
		restartCount++
		return nil
	}

	// healthCheck проверяет, что хром target отвечает на CDP команды (5s timeout)
	// Если не отвечает → target завис, нужен restartSession
	healthCheck := func() bool {
		if ctx == nil {
			return false
		}
		checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
		defer checkCancel()
		var loc string
		err := chromedp.Run(checkCtx, chromedp.Location(&loc))
		if err != nil {
			log.Printf("MonitorAPI: health check failed: %v (ctx.Err=%v)", err, ctx.Err())
			return false
		}
		return true
	}

	// Первая сессия
	if err := startSession(); err != nil {
		log.Printf("MonitorAPI: init session: %v", err)
		if tel != nil {
			tel.Sendf("❌ MonitorAPI: init session: %v", err)
		}
		return
	}
	defer func() {
		if session != nil {
			session.Close()
		}
	}()

	interval := 360 * time.Second
	autoSign := true
	giveUp := map[string]bool{}

	fetchFails := 0
	lastFetchAlert := time.Time{}
	tickCount := 0 // каждых 3 обход очищаем giveUp

	// Профилактический рестарт каждые 24 часа (деградация хрома/CDP со временем)
	const prophylacticRestartInterval = 24 * time.Hour
	lastProphylacticRestart := time.Now()

	log.Println("MonitorAPI запущен. Тикер:", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	resetCh := make(chan struct{}, 1)

	// Один полный обход сразу при старте.
	checkAndSign := func() {
		tickCount++

		// Профилактический рестарт каждые 24 часа
		if time.Since(lastProphylacticRestart) > prophylacticRestartInterval {
			log.Printf("MonitorAPI: профилактический рестарт (24h)")
			if err := restartSession("prophylactic 24h"); err != nil {
				log.Printf("MonitorAPI: prophylactic restart failed: %v", err)
				return
			}
			lastProphylacticRestart = time.Now()
			// После рестарта пропускаем этот тик, следующий будет через interval
			return
		}

		// Health-check перед каждым тиком: если хром завис → рестарт сессии
		if !healthCheck() {
			if err := restartSession("health check failed"); err != nil {
				log.Printf("MonitorAPI: restart after health check failed: %v", err)
				return
			}
			// После рестарта пробуем еще раз в этом тике
		}

		if tickCount%3 == 0 {
			giveUp = map[string]bool{}
			log.Println("MonitorAPI: очистка giveUp каждые 3 тика")
			tickCount = 0

		}
		notes, err := FetchNotesAPI(ctx, 30)
		if err != nil {
			log.Printf("MonitorAPI: ошибка получения накладных: %v", err)

			lastFetchMu.Lock()
			lastFetchError = err.Error()
			lastFetchTime = time.Now()
			lastFetchMu.Unlock()

			fetchFails++
			errStr := err.Error()

			// Немедленный алерт на критические ошибки:
			// reload не выполнился, context отменён, браузер завис.
			isCritical := strings.Contains(errStr, "reload:") ||
				strings.Contains(errStr, "context cancelled") ||
				strings.Contains(errStr, "общий таймаут")

			if tel != nil && isCritical {
				tel.Sendf("🔴 MonitorAPI: критическая ошибка: %v", err)
			}

			// Эскалация при повторяющихся таймаутах перехвата.
			if tel != nil &&
				!isCritical &&
				fetchFails >= 3 &&
				time.Since(lastFetchAlert) >= 10*time.Minute {

				lastFetchAlert = time.Now()

				tel.Sendf(
					"⚠️ Мониторинг: не получаю накладные %d раз подряд: %v",
					fetchFails,
					err,
				)
			}

			return
		}

		fetchFails = 0

		lastFetchMu.Lock()
		lastFetchTime = time.Now()
		lastNotes = append([]DeliveryNote(nil), notes...)
		lastFetchError = ""
		lastFetchMu.Unlock()

		log.Printf("MonitorAPI: накладные (%d):", len(notes))

		for _, n := range notes {
			log.Printf(
				"  %s  от %s  %s → %s  %s",
				n.Number,
				n.Date,
				n.Consignor,
				n.Consignee,
				n.Carrier,
			)
		}

		// Накладные, которых больше нет в списке,
		// разблокируем.
		//
		// Если они снова появятся позже —
		// можно будет попробовать подписать их ещё раз.
		for num := range giveUp {
			found := false

			for _, n := range notes {
				if n.Number == num {
					found = true
					break
				}
			}

			if !found {
				delete(giveUp, num)
			}
		}

		// Получаем накладные, доступные для подписания.
		todo := signableNotes(notes)

		// Убираем накладные, которые ранее окончательно
		// не удалось подписать.
		filtered := todo[:0]

		for _, n := range todo {
			if !giveUp[n.Number] {
				filtered = append(filtered, n)
			}
		}

		todo = filtered

		if len(todo) > 0 && tel != nil {
			nums := make([]string, 0, len(todo))

			for _, n := range todo {
				nums = append(nums, n.Number)
			}

			tel.Sendf(
				"ВНИМАНИЕ: накладные на подпись (%d): %v",
				len(todo),
				nums,
			)
		}

		// ВАЖНО:
		//
		// autoSign берётся из текущего состояния.
		// Если AutoSign изменился через cmdCh во время
		// предыдущего обхода, новое значение будет применено
		// именно здесь, на следующем обходе.
		if autoSign && len(todo) > 0 {
			if err := signAllAPI(
				ctx,
				cfg.CertUser,
				todo,
				tel,
				giveUp,
			); err != nil {

				log.Printf(
					"MonitorAPI: SignAll ошибка: %v",
					err,
				)

				if tel != nil {
					tel.Sendf(
						"❌ MonitorAPI: SignAll ошибка: %v",
						err,
					)
				}
			} else {
				log.Println("MonitorAPI: SignAll завершено")

				if tel != nil {
					tel.Sendf("MonitorAPI: SignAll завершено")
				}
			}
		}
	}

	// =========================================================
	// ПЕРВЫЙ ОБХОД СРАЗУ ПРИ СТАРТЕ
	// =========================================================

	checkAndSign()

	stopAnnouncer := startAnnouncer(ctx, interval, resetCh)
	defer stopAnnouncer()

	announce(resetCh)
	// =========================================================
	// ОСНОВНОЙ ЦИКЛ
	// =========================================================

	for {
		select {

		// -----------------------------------------------------
		// Команды управления
		// -----------------------------------------------------

		case cmd, ok := <-cmdCh:
			if !ok {
				log.Println(
					"MonitorAPI: канал управления закрыт, завершаюсь",
				)
				return
			}

			// Меняем интервал.
			//
			// Новый интервал будет использоваться
			// начиная со следующего тика.
			if cmd.Interval > 0 {
				interval = cmd.Interval

				ticker.Reset(interval)

				stopAnnouncer()

				stopAnnouncer = startAnnouncer(
					ctx,
					interval,
					resetCh,
				)

				announce(resetCh)
			}

			// Меняем AutoSign.
			//
			// Если сейчас signAllAPI уже выполняется,
			// оно НЕ будет прервано.
			//
			// Новое значение будет использовано
			// при следующем checkAndSign().
			if cmd.AutoSign != nil {
				autoSign = *cmd.AutoSign
			}

			log.Printf(
				"MonitorAPI: параметры обновлены (interval=%s, autosign=%v)",
				interval,
				autoSign,
			)

		// -----------------------------------------------------
		// Очередной обход
		// -----------------------------------------------------

		case <-ticker.C:

			checkAndSign()
			announce(resetCh)
		}
	}
}
