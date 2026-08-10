package SCRP

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
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
// /transportations: RequestID -> true если это сам список.
// pausedAt хранит время паузы каждого перехваченного запроса — чтобы из
// карты можно было выбрать самый свежий, а не случайный.
var (
	pendingMu sync.Mutex
	pending   = map[fetch.RequestID]bool{}
	pausedAt  = map[fetch.RequestID]time.Time{}
)

// startTransportationsCapture включает fetch-домен и перехватывает в
// response-stage запросы, чей URL заканчивается на /transportations.
// Запрос удерживается в паузе до обработки в основном потоке
// (fetch.GetResponseBody + fetch.ContinueResponse) — это самый надёжный
// способ получить тело ответа SPA вместе со свежими токенами.
func startTransportationsCapture(ctx context.Context) {
	pendingMu.Lock()
	pending = map[fetch.RequestID]bool{}
	pausedAt = map[fetch.RequestID]time.Time{}
	pendingMu.Unlock()

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		return fetch.Enable().
			WithPatterns([]*fetch.RequestPattern{{
				URLPattern:   "*transportations",
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
		isList := false
		if e.Request != nil {
			isList = strings.HasSuffix(e.Request.URL, "/transportations")
		}
		pendingMu.Lock()
		pending[e.RequestID] = isList
		pausedAt[e.RequestID] = time.Now()
		pendingMu.Unlock()
	})
}

// reloadNotesAPI перезагружает страницу, заставляя SPA сделать свежий
// запрос /transportations (актуальные токены), и даёт 3с на старт.
func reloadNotesAPI(ctx context.Context) {
	chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCacheDisabled(true).Do(ctx)
	}))
	chromedp.Run(ctx, chromedp.Reload())
	chromedp.Run(ctx, chromedp.Sleep(3*time.Second))
}

// FetchNotesAPI получает список накладных, перехватив ответ SPA.
// timeoutSec — сколько секунд ждём появления/тела ответа.
func FetchNotesAPI(ctx context.Context, timeoutSec int) ([]DeliveryNote, error) {
	notes, err := fetchNotesAPICtx(ctx, timeoutSec)
	if err != nil && strings.Contains(err.Error(), "Invalid InterceptionId") {
		log.Printf("FetchNotesAPI: протухший перехват, повторяю (ретрая)...")
		continuePaused(ctx)
		notes, err = fetchNotesAPICtx(ctx, timeoutSec)
	}
	return notes, err
}

func fetchNotesAPICtx(ctx context.Context, timeoutSec int) ([]DeliveryNote, error) {
	// Сбрасываем перехваченные запросы перед перезагрузкой: навигация отменяет
	// висевшие паузные запросы (их InterceptionId становится невалидным), и они
	// не должны оставаться кандидатами на выборку.
	pendingMu.Lock()
	pending = map[fetch.RequestID]bool{}
	pausedAt = map[fetch.RequestID]time.Time{}
	pendingMu.Unlock()

	reloadNotesAPI(ctx)

	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)

	// Ждём появления паузнутого запроса списка.
	var listID fetch.RequestID
	for listID == "" && time.Now().Before(deadline) {
		pendingMu.Lock()
		var newest time.Time
		for id, isList := range pending {
			if isList {
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
			chromedp.Run(ctx, chromedp.Sleep(400*time.Millisecond))
		}
	}
	if listID == "" {
		continuePaused(ctx)
		return nil, fmt.Errorf("не перехвачен запрос /transportations за %ds", timeoutSec)
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
	pending = map[fetch.RequestID]bool{}
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
	session := browser.NewSession()
	defer session.Close()

	ctx := session.Ctx()

	if err := initSession(ctx, cfg); err != nil {
		log.Printf("MonitorAPI: init session: %v", err)
		if tel != nil {
			tel.Sendf("❌ MonitorAPI: init session: %v", err)
		}
		return
	}

	startTransportationsCapture(ctx)

	interval := 360 * time.Second
	autoSign := true
	giveUp := map[string]bool{}

	fetchFails := 0
	lastFetchAlert := time.Time{}

	log.Println("MonitorAPI запущен. Тикер:", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	resetCh := make(chan struct{}, 1)
	stopAnnouncer := startAnnouncer(ctx, interval, resetCh)
	announce(resetCh)

	for {
		select {
		case cmd, ok := <-cmdCh:
			if !ok {
				log.Println("MonitorAPI: канал управления закрыт, завершаюсь")
				return
			}
			if cmd.Interval > 0 {
				interval = cmd.Interval
				ticker.Reset(interval)
				stopAnnouncer()
				stopAnnouncer = startAnnouncer(ctx, interval, resetCh)
				announce(resetCh)
			}
			if cmd.AutoSign != nil {
				autoSign = *cmd.AutoSign
			}
			log.Printf("MonitorAPI: параметры обновлены (interval=%s, autosign=%v)", interval, autoSign)
		case <-ticker.C:
			announce(resetCh)

			notes, err := FetchNotesAPI(ctx, 30)
			if err != nil {
				log.Printf("MonitorAPI: ошибка получения накладных: %v", err)
				fetchFails++
				if tel != nil && fetchFails >= 3 && time.Since(lastFetchAlert) >= 10*time.Minute {
					lastFetchAlert = time.Now()
					tel.Sendf("⚠️ Мониторинг: не получаю накладные %d раз подряд: %v", fetchFails, err)
				}
				continue
			}
			fetchFails = 0

			log.Printf("MonitorAPI: накладные (%d):", len(notes))
			for _, n := range notes {
				log.Printf("  %s  от %s  %s → %s  %s", n.Number, n.Date, n.Consignor, n.Consignee, n.Carrier)
			}

			// Накладные, которых больше нет в списке, разблокируем (если
			// вернутся позже — можно попробовать снова).
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

			todo := signableNotes(notes)
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
				tel.Sendf("ВНИМАНИЕ: накладные на подпись (%d): %v", len(todo), nums)
			}

			if autoSign && len(todo) > 0 {
				if err := signAllAPI(ctx, cfg.CertUser, todo, tel, giveUp); err != nil {
					log.Printf("MonitorAPI: SignAll ошибка: %v", err)
					if tel != nil {
						tel.Sendf("❌ MonitorAPI: SignAll ошибка: %v", err)
					}
				} else {
					log.Println("MonitorAPI: SignAll завершено")
					if tel != nil {
						tel.Sendf("MonitorAPI: SignAll завершено")
					}
				}
			}
		}
	}
}
