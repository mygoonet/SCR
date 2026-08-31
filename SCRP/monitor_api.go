package SCRP

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	neturl "net/url"
	"strconv"
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

// apiPossibleAction — элемент possibleActions накладной.
type apiPossibleAction struct {
	Name        string `json:"name"`
	IsAvailable bool   `json:"isAvailable"`
}

// apiPerson — подписант (sender/recipient) в метаданных.
type apiPerson struct {
	ActionDate         string `json:"actionDate"`
	IsRejected         bool   `json:"isRejected"`
	IsInvalidSignature bool   `json:"isInvalidSignature"`
	Position           string `json:"position"`
	FirstName          string `json:"firstName"`
	LastName           string `json:"lastName"`
	MiddleName         string `json:"middleName"`
}

// apiPlannedWeight — планируемый вес позиции груза.
type apiPlannedWeight struct {
	GrossWeight float64 `json:"grossWeight"`
}

// apiCargoItem — позиция груза в cargoInfo.
type apiCargoItem struct {
	Name                string            `json:"name"`
	CodeTnved           string            `json:"codeTnved"`
	Condition           string            `json:"condition"`
	PackageMethod       string            `json:"packageMethod"`
	ContainerType       string            `json:"containerType"`
	CargoSpaceQuantity  int               `json:"cargoSpaceQuantity"`
	Marks               []string          `json:"marks"`
	DangerousItems      []json.RawMessage `json:"dangerousItems"`
	TransportContainers []json.RawMessage `json:"transportContainers"`
	PlannedWeight       apiPlannedWeight  `json:"plannedWeight"`
}

// apiCargoInfo — блок cargoInfo в метаданных.
type apiCargoInfo struct {
	Items []apiCargoItem `json:"items"`
}

// apiPowerOfAttorney — доверенность получателя (recipientTitleInfo).
type apiPowerOfAttorney struct {
	StatusVerification struct {
		StatusType string `json:"statusType"`
	} `json:"statusVerification"`
}

// apiRecipientTitleInfo — recipientTitleInfo в метаданных.
type apiRecipientTitleInfo struct {
	PowerOfAttorney apiPowerOfAttorney `json:"powerOfAttorney"`
}

// apiReceptionMetadata — метаданные приёмки/сдачи (receptionMetadata,
// deliveryMetadata и элементы *RevisionsMetadata имеют одинаковую форму).
type apiReceptionMetadata struct {
	ID                       string                `json:"id"`
	TitleVersion             string                `json:"titleVersion"`
	ActualCargoCondition     string                `json:"actualCargoCondition"`
	PlannedShippingDate      string                `json:"plannedShippingDate"`
	ActionStartedAt          string                `json:"actionStartedAt"`
	ActionFinishedAt         string                `json:"actionFinishedAt"`
	ActionStartedAtUtcOffset string                `json:"actionStartedAtUtcOffset"`
	NumberOfCargoSpaces      string                `json:"numberOfCargoSpaces"`
	MassBrutto               string                `json:"massBrutto"`
	MassBruttoUnits          string                `json:"massBruttoUnits"`
	Sender                   apiPerson             `json:"sender"`
	Recipient                apiPerson             `json:"recipient"`
	TruckInfo                string                `json:"truckInfo"`
	ControllerResolutions    []json.RawMessage     `json:"controllerResolutions"`
	CargoInfo                apiCargoInfo          `json:"cargoInfo"`
	SenderTitleInfo          json.RawMessage       `json:"senderTitleInfo"`
	RecipientTitleInfo       apiRecipientTitleInfo `json:"recipientTitleInfo"`
	HasConsigneeNotes        bool                  `json:"hasConsigneeNotes"`
	DeliveryType             string                `json:"deliveryType"`
}

// apiAttachmentDocuments — вложения накладной.
type apiAttachmentDocuments struct {
	Attachments []json.RawMessage `json:"attachments"`
}

type apiTransportation struct {
	ID            string `json:"id"`
	WaybillNumber string `json:"waybillNumber"`
	WaybillDate   string `json:"waybillDate"`
	OrderNumber   string `json:"orderNumber"`
	OrderDate     string `json:"orderDate"`
	CargoName     string `json:"cargoName"`

	ConsignorName    string `json:"consignorName"`
	ConsignorAddress string `json:"consignorAddress"`
	ReceptionAddress string `json:"receptionAddress"`

	ConsigneeName    string `json:"consigneeName"`
	ConsigneeAddress string `json:"consigneeAddress"`
	DeliveryAddress  string `json:"deliveryAddress"`

	CarrierName    string `json:"carrierName"`
	CarrierAddress string `json:"carrierAddress"`

	CurrentDriver apiDriver `json:"currentDriver"`
	TruckInfo     string    `json:"truckInfo"`

	ReceptionMetadata          *apiReceptionMetadata  `json:"receptionMetadata"`
	DeliveryMetadata           *apiReceptionMetadata  `json:"deliveryMetadata"`
	ReceptionRevisionsMetadata []apiReceptionMetadata `json:"receptionRevisionsMetadata"`
	DeliveryRevisionsMetadata  []apiReceptionMetadata `json:"deliveryRevisionsMetadata"`
	RelayMetadata              []json.RawMessage      `json:"relayMetadata"`
	ReaddressMetadata          []json.RawMessage      `json:"readdressMetadata"`

	IsObservable         bool   `json:"isObservable"`
	IsTestTransportation bool   `json:"isTestTransportation"`
	Status               string `json:"status"`
	StatusText           string `json:"statusText"`
	CreatedAt            string `json:"createdAt"`
	ModifiedAt           string `json:"modifiedAt"`

	PossibleActions []apiPossibleAction `json:"possibleActions"`

	IsDraftForOtherOrg          bool `json:"isDraftForOtherOrg"`
	IsReceptionRevisionUnsigned bool `json:"isReceptionRevisionUnsigned"`
	IsDeliveryRevisionUnsigned  bool `json:"isDeliveryRevisionUnsigned"`
	IsDeliveredPartially        bool `json:"isDeliveredPartially"`
	IsDeliveryRejected          bool `json:"isDeliveryRejected"`

	AttachmentDocuments     apiAttachmentDocuments `json:"attachmentDocuments"`
	ConsigneeAdditionalInfo []json.RawMessage      `json:"consigneeAdditionalInfo"`
	ConsignorAdditionalInfo []json.RawMessage      `json:"consignorAdditionalInfo"`
	FormatVersion           string                 `json:"formatVersion"`
}

type apiTransportations struct {
	Items          []apiTransportation `json:"items"`
	HasMoreResults bool                `json:"hasMoreResults"`
	TotalCount     int                 `json:"totalCount"`
	CurrentPage    int                 `json:"currentPage"`
	TotalPages     int                 `json:"totalPages"`
}

// captureState — состояние fetch-перехвата, приватное для одной сессии.
// pending хранит перехваченные (паузнутые в response-stage) запросы к
// /transportations: RequestID -> URL запроса.
// pausedAt хранит время паузы каждого перехваченного запроса — чтобы из
// карты можно было выбрать самый свежий, а не случайный.
type captureState struct {
	mu       sync.Mutex
	pending  map[fetch.RequestID]string
	pausedAt map[fetch.RequestID]time.Time
}

func newCaptureState() *captureState {
	return &captureState{
		pending:  map[fetch.RequestID]string{},
		pausedAt: map[fetch.RequestID]time.Time{},
	}
}

func (cs *captureState) add(id fetch.RequestID, url string) {
	cs.mu.Lock()
	cs.pending[id] = url
	cs.pausedAt[id] = time.Now()
	cs.mu.Unlock()
}

// newestListRequest ищет самый свежий паузнутый запрос /transportations.
func (cs *captureState) newestListRequest() (fetch.RequestID, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	var (
		listID fetch.RequestID
		newest time.Time
		found  bool
	)
	for id, url := range cs.pending {
		u, err := neturl.Parse(url)
		if err != nil || !strings.HasSuffix(u.Path, "/transportations") {
			continue
		}
		if !found || cs.pausedAt[id].After(newest) {
			listID, newest, found = id, cs.pausedAt[id], true
		}
	}
	return listID, found
}

// drainAll вынимает и очищает все id, которые нужно отпустить.
func (cs *captureState) drainAll() []fetch.RequestID {
	cs.mu.Lock()
	ids := make([]fetch.RequestID, 0, len(cs.pending))
	for id := range cs.pending {
		ids = append(ids, id)
	}
	cs.pending = map[fetch.RequestID]string{}
	cs.pausedAt = map[fetch.RequestID]time.Time{}
	cs.mu.Unlock()
	return ids
}

// snapshotURLs — для логов при таймауте.
func (cs *captureState) snapshotURLs() []string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	urls := make([]string, 0, len(cs.pending))
	for _, u := range cs.pending {
		urls = append(urls, u)
	}
	return urls
}

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
// Возвращает captureState, привязанный к этой сессии/ctx: весь последующий
// код (FetchNotesAPI/continuePaused) для этой сессии должен использовать
// именно его, а не какое-либо общее состояние.
func startTransportationsCapture(ctx context.Context) *captureState {
	cs := newCaptureState()

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
				go func(parent context.Context) {
					bg, cancel := context.WithTimeout(parent, 5*time.Second)
					defer cancel()
					if err := chromedp.Run(bg, chromedp.ActionFunc(func(c context.Context) error {
						return fetch.ContinueResponse(reqID).Do(c)
					})); err != nil {
						log.Printf("fetch-intercept: continue negotiate/etc failed: %v", err)
					}
				}(ctx)
				return
			}
		}
		log.Printf("fetch-intercept: %s", url)

		cs.add(e.RequestID, url)
	})

	return cs
}

// reloadNotesAPI перезагружает страницу, заставляя SPA сделать свежий
// запрос /transportations (актуальные токены), и даёт 5с на старт.
// Если текущая страница - waybill (/sign/waybill), Reload бесполезен (лог 10:11:42 location=.../waybill
// -> pending только negotiate) - делаем быстрый возврат на список без waitForTableRows
// (waitForTableRows под паузой fetch дедлочит 30с).
// SetCacheDisabled уже был сделан в initSession (SCRP/scraper.go:143) навсегда - здесь не нужен.
func reloadNotesAPI(ctx context.Context, cs *captureState) error {
	start := time.Now()
	var loc string
	if err := chromedp.Run(ctx, chromedp.Location(&loc)); err != nil {
		log.Printf("reloadNotesAPI: Location error: %v (ctx.Err=%v)", err, ctx.Err())
		return fmt.Errorf("location: %w", err)
	}
	log.Printf("reloadNotesAPI: start loc=%s ctx.Err=%v", loc, ctx.Err())
	if strings.Contains(loc, "/sign/") || strings.Contains(loc, "/waybill") {
		log.Printf("reloadNotesAPI: на waybill %s -> быстрый возврат на список", loc)
		// Продолжаем все паузы чтобы разблокировать SPA перед навигацией
		continuePaused(ctx, cs)
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
				if navErr := chromedp.Run(ctx, chromedp.Navigate(listURL)); navErr != nil {
					log.Printf("reloadNotesAPI: Navigate error: %v (ctx.Err=%v)", navErr, ctx.Err())
				}
			} else {
				if navErr := chromedp.Run(ctx, chromedp.NavigateBack()); navErr != nil {
					log.Printf("reloadNotesAPI: NavigateBack error: %v (ctx.Err=%v)", navErr, ctx.Err())
				}
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
		return fmt.Errorf("reload: %w (ctx.Err=%v, duration=%v)", err, ctx.Err(), time.Since(start))
	}
	if err := chromedp.Run(ctx, chromedp.Sleep(5*time.Second)); err != nil {
		return fmt.Errorf("sleep after reload: %w", err)
	}
	log.Printf("reloadNotesAPI: обычный путь завершен, duration=%v", time.Since(start))
	return nil
}

// FetchNotesAPI получает список накладных, перехватив ответ SPA.
// timeoutSec — сколько секунд ждём появления/тела ответа.
// Общий таймаут = timeoutSec + 30с на каждую попытку (запас на reload+навигацию waybill+GetResponseBody),
// ретрай на свежем контексте иначе вторая попытка ловит deadline exceeded как в 10:11:52 и 11:19:37.
func FetchNotesAPI(ctx context.Context, cs *captureState, timeoutSec int) ([]DeliveryNote, error) {
	withTimeout := func(parent context.Context) (context.Context, context.CancelFunc) {
		return context.WithTimeout(parent, time.Duration(timeoutSec+30)*time.Second)
	}

	outerCtx, outerCancel := withTimeout(ctx)
	notes, err := fetchNotesAPICtx(outerCtx, cs, timeoutSec)
	outerCancel()
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "Invalid InterceptionId") {
			log.Printf("FetchNotesAPI: протухший перехват, повторяю (ретрай на свежем контексте)...")
			continuePaused(ctx, cs)
			// пауза 2s: хром/SPA должны успокоиться после неудачной попытки
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
			retryCtx, retryCancel := withTimeout(ctx)
			defer retryCancel()
			notes, err = fetchNotesAPICtx(retryCtx, cs, timeoutSec)
		} else if strings.Contains(errStr, "не перехвачен запрос") || strings.Contains(errStr, "reload") {
			log.Printf("FetchNotesAPI: таймаут/Reload ошибка, повторяю (ретрай на свежем контексте)...")
			continuePaused(ctx, cs)
			// увеличенная пауза 3s: хром мог зависнуть на SetCacheDisabled/Reload
			chromedp.Run(ctx, chromedp.Sleep(3*time.Second))
			retryCtx, retryCancel := withTimeout(ctx)
			defer retryCancel()
			notes, err = fetchNotesAPICtx(retryCtx, cs, timeoutSec)
		} else if ctx.Err() != nil {
			log.Printf("FetchNotesAPI: общий таймаут (browser завис?): %v", err)
		} else if outerCtx.Err() != nil {
			log.Printf("FetchNotesAPI: таймаут попытки: %v", err)
		}
	}
	return notes, err
}

func fetchNotesAPICtx(ctx context.Context, cs *captureState, timeoutSec int) ([]DeliveryNote, error) {
	// Отпускаем зависшие паузы и очищаем stale-кандидатов перед перезагрузкой:
	// навигация отменяет висевшие паузные запросы (их InterceptionId становится
	// невалидным), и они не должны оставаться кандидатами на выборку.
	continuePaused(ctx, cs)

	if err := reloadNotesAPI(ctx, cs); err != nil {
		return nil, fmt.Errorf("reload: %w", err)
	}

	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)

	// Ждём появления паузнутого запроса списка.
	var listID fetch.RequestID
	for listID == "" && time.Now().Before(deadline) {
		if ctx.Err() != nil {
			continuePaused(ctx, cs)
			return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
		}
		// Из карты берём самый свежий запрос: старые могли быть
		// отменены навигацией (Invalid InterceptionId).
		if id, ok := cs.newestListRequest(); ok {
			listID = id
		}
		if listID == "" {
			if err := chromedp.Run(ctx, chromedp.Sleep(400*time.Millisecond)); err != nil {
				if ctx.Err() != nil {
					continuePaused(ctx, cs)
					return nil, fmt.Errorf("context cancelled during wait: %w", ctx.Err())
				}
				log.Printf("FetchNotesAPI: sleep error: %v", err)
				// Защита от быстрого спина при потере CDP-коннекта:
				// chromedp.Sleep упал, но ctx жив — ждём обычным time.Sleep.
				time.Sleep(400 * time.Millisecond)
			}
		}
	}
	if listID == "" {
		urls := cs.snapshotURLs()

		var loc string
		chromedp.Run(ctx, chromedp.Location(&loc))

		log.Printf("FetchNotesAPI: таймаут %ds, pending=%d, urls=%v, location=%s",
			timeoutSec, len(urls), urls, loc)

		continuePaused(ctx, cs)
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
		continuePaused(ctx, cs)
		return nil, fmt.Errorf("get response body: %w", err)
	}

	// Отпускаем все паузнутые (и сам список, и прочие совпадения).
	continuePaused(ctx, cs)

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
			Number:           it.WaybillNumber,
			Date:             it.WaybillDate,
			Consignor:        it.ConsignorName,
			ConsignorAddress: it.ConsignorAddress,
			Consignee:        it.ConsigneeName,
			ConsigneeAddress: it.ConsigneeAddress,
			DeliveryAddress:  it.DeliveryAddress,
			Carrier:          it.CarrierName,
			Driver:           driverFullName(it.CurrentDriver),
			DriverPhone:      it.CurrentDriver.Phone,
			Truck:            it.TruckInfo,
			CreatedAt:        formatCreatedAt(it.CreatedAt),
		}
		if n.DeliveryAddress == "" {
			n.DeliveryAddress = it.ReceptionAddress
		}
		notes = append(notes, n)
	}
	return notes, nil
}

// continuePaused отпускает все перехваченные запросы, чтобы страница не висела.
// Если ctx уже мёртв (timeout/cancel) — CDP-команды на нём не пройдут и паузы
// останутся висеть; в этом случае используем свежий фоновый контекст с 5s таймаутом.
func continuePaused(ctx context.Context, cs *captureState) {
	if ctx.Err() != nil {
		bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx = bg
	}
	ids := cs.drainAll()

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

// formatCreatedAt переводит createdAt из API в формат timeLayout ("2006-01-02 15:04:05").
// Поддерживает RFC3339 (UTC, конвертируется в локальное время) и человекочитаемый
// формат "24 августа 16:09 (UTC +08:00)" (год — текущий). При ошибке парсинга
// возвращает исходную строку.
func formatCreatedAt(s string) string {
	if s == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.Local().Format(timeLayout)
	}
	if t, ok := parseHumanDate(s); ok {
		return t.Format(timeLayout)
	}
	return s
}

var ruMonthNames = map[string]int{
	"января": 1, "февраля": 2, "марта": 3, "апреля": 4, "мая": 5, "июня": 6,
	"июля": 7, "августа": 8, "сентября": 9, "октября": 10, "ноября": 11, "декабря": 12,
}

// parseHumanDate разбирает "24 августа 16:09 (UTC +08:00)" — время берётся как
// есть (стеночное время сервера), год подставляется текущий.
func parseHumanDate(s string) (time.Time, bool) {
	fields := strings.Fields(s)
	if len(fields) < 3 {
		return time.Time{}, false
	}
	day, err := strconv.Atoi(fields[0])
	if err != nil || day < 1 || day > 31 {
		return time.Time{}, false
	}
	month, ok := ruMonthNames[fields[1]]
	if !ok {
		return time.Time{}, false
	}
	var h, m int
	if _, err := fmt.Sscanf(fields[2], "%d:%d", &h, &m); err != nil || h > 23 || m > 59 {
		return time.Time{}, false
	}
	now := time.Now()
	return time.Date(now.Year(), time.Month(month), day, h, m, 0, 0, time.Local), true
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

	cs := startTransportationsCapture(ctx)

	notes, err := FetchNotesAPI(ctx, cs, 20)
	if err != nil {
		log.Printf("TestAPI: fetch: %v", err)
		return
	}

	log.Printf("TestAPI: накладных %d:", len(notes))
	for _, n := range notes {
		log.Printf("  %s  от %s  создан %s  %s → %s  %s", n.Number, n.Date, n.CreatedAt, n.Consignor, n.Consignee, n.Carrier)
	}
}

// signAllAPI — подписание с проверкой исчезновения через API-перехват
// (FetchNotesAPI), а не через DOM. DOM-проверка (reloadNoCache +
// waitForTableRows + fetchNotes) здесь не работает: fetch-перехват держит
// запросы /transportations в паузе, страница не дорисовывает таблицу, и
// DOM-чтение даёт 0 строк, из-за чего цикл верификации висит.
// giveUp — накладные, которые не подписаны после 3 попыток: они скипаются
// до конца сессии (пока не уйдут из списка), чтобы не крутить цикл вечно.
func signAllAPI(ctx context.Context, cs *captureState, certUser string, notes []DeliveryNote, tel *TelegramClient, giveUp map[string]bool) error {
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
				if _, refreshErr := FetchNotesAPI(ctx, cs, 20); refreshErr != nil {
					nl.Logf(">>> refresh после ошибки подписания: %v", refreshErr)
				}
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
				refreshed, err := FetchNotesAPI(ctx, cs, 20)
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
			// Не даём слайсу расти бесконечно — держим последние 50.
			if len(signingFailures) > 50 {
				signingFailures = signingFailures[len(signingFailures)-50:]
			}
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
	var cs *captureState
	sessionStartedAt := time.Now()
	restartCount := 0

	// startSession создает новую сессию, инициализирует и включает fetch-перехват.
	// captureState создаётся заново на каждую сессию: RequestID из старой
	// (закрытой) сессии не должны попадать в pending новой.
	startSession := func() error {
		session = browser.NewSession()
		ctx = session.Ctx()
		if err := initSession(ctx, cfg); err != nil {
			session.Close()
			return fmt.Errorf("init session: %w", err)
		}
		cs = startTransportationsCapture(ctx)
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
		notes, err := FetchNotesAPI(ctx, cs, 30)
		if err != nil {
			log.Printf("MonitorAPI: ошибка получения накладных: %v", err)

			lastFetchMu.Lock()
			lastFetchError = err.Error()
			lastFetchTime = time.Now()
			lastFetchMu.Unlock()

			fetchFails++
			errStr := err.Error()

			// Немедленный алерт на критические ошибки:
			// reload не выполнился, context отменён.
			isCritical := strings.Contains(errStr, "reload:") ||
				strings.Contains(errStr, "context cancelled")

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
				"  %s  от %s  создан %s  %s → %s  %s",
				n.Number,
				n.Date,
				n.CreatedAt,
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
		filtered := make([]DeliveryNote, 0, len(todo))

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
				cs,
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
