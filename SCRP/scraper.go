package SCRP

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Scraper — обёртка над Session для периодического опроса и подписания накладных.
type Scraper struct {
	session   *Session
	signCh    chan string
	signAllCh chan chan error
	listCh    chan chan []DeliveryNote
}

// NewScraper создаёт новый скрепер с заданной конфигурацией.
func NewScraper(cfg Config) *Scraper {
	return &Scraper{
		session:   New(cfg),
		signCh:    make(chan string, 10),
		signAllCh: make(chan chan error, 1),
		listCh:    make(chan chan []DeliveryNote, 1),
	}
}

// Start запускает цикл: каждые 50 секунд выводит список и подписывает все накладные.
func (sc *Scraper) Start() error {
	if err := sc.session.Open(); err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	defer sc.session.Close()

	ctx := sc.session.Ctx()

	if err := sc.initSession(ctx); err != nil {
		return fmt.Errorf("init session: %w", err)
	}

	log.Println("Scraper запущен. Тикер 50с: вывод + автоподпись всех")

	// Сразу получаем накладные при старте
	notes, err := sc.fetchNotes(ctx)
	if err != nil {
		log.Printf("Ошибка получения накладных: %v", err)
	} else {
		log.Printf("Накладные (%d):", len(notes))
		for _, n := range notes {
			log.Printf("  %s  от %s  %s → %s  %s", n.Number, n.Date, n.Consignor, n.Consignee, n.Carrier)
		}
	}

	ticker := time.NewTicker(600 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case number := <-sc.signCh:
			log.Printf(">>> Подписание накладной %s...", number)
			go sc.signOne(ctx, number)

		case listResp := <-sc.listCh:
			notes, err := sc.fetchNotes(ctx)
			if err != nil {
				log.Printf("Ошибка получения накладных: %v", err)
				listResp <- nil
			} else {
				listResp <- notes
			}

		case respCh := <-sc.signAllCh:
			go func() {
				respCh <- sc.signAllInCtx(ctx)
			}()

		case <-ticker.C:
			notes, err := sc.fetchNotes(ctx)
			if err != nil {
				log.Printf("Ошибка получения накладных: %v", err)
			} else {
				log.Printf("Накладные (%d):", len(notes))
				for _, n := range notes {
					log.Printf("  %s  от %s  %s → %s  %s", n.Number, n.Date, n.Consignor, n.Consignee, n.Carrier)
				}

				if len(notes) > 0 {
					if err := sc.signAll(notes); err != nil {
						log.Printf(">>> SignAll ошибка: %v", err)
					} else {
						log.Println(">>> SignAll завершено")
					}
				}
			}
		}
	}
}

func (sc *Scraper) signOne(ctx context.Context, number string) {
	if err := SignDeliveryNote(ctx, number, sc.session.Cfg().CertUser); err != nil {
		log.Printf(">>> Ошибка подписания %s: %v", number, err)
	} else {
		log.Printf(">>> Накладная %s подписана", number)
	}
}

func (sc *Scraper) signAllInCtx(ctx context.Context) error {
	notes, err := sc.fetchNotes(ctx)
	if err != nil {
		return fmt.Errorf("получить список: %w", err)
	}
	return sc.signAll(notes)
}

var skipNumbers = map[string]bool{
	"000000420": true,
}

func (sc *Scraper) signAll(notes []DeliveryNote) error {
	ctx := sc.session.Ctx()
	cert := sc.session.Cfg().CertUser
	var firstErr error
	for _, n := range notes {
		if skipNumbers[n.Number] {
			log.Printf(">>> SignAll: пропускаю %s (тестовая)", n.Number)
			continue
		}
		signed := false
		for attempt := 1; attempt <= 3; attempt++ {
			log.Printf(">>> SignAll: подписываю %s (попытка %d)...", n.Number, attempt)
			if err := SignDeliveryNote(ctx, n.Number, cert); err != nil {
				log.Printf(">>> Ошибка подписания %s: %v", n.Number, err)
				if firstErr == nil {
					firstErr = err
				}
				closePopups(ctx)
				reloadNoCache(ctx)
				waitForTableRows(ctx)
				continue
			}
			log.Printf(">>> Накладная %s подписана, жду обновления страницы...", n.Number)
			closePopups(ctx)
			chromedp.Run(ctx, chromedp.Sleep(5*time.Second))
			reloadNoCache(ctx)
			waitForTableRows(ctx)
			chromedp.Run(ctx, chromedp.Sleep(3*time.Second))
			refreshed, err := sc.fetchNotes(ctx)
			if err != nil {
				log.Printf(">>> Ошибка обновления списка: %v", err)
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
			log.Printf(">>> Накладная %s ещё в списке, повторяю...", n.Number)
		}
		if !signed {
			log.Printf(">>> Накладная %s не подписана после 3 попыток", n.Number)
		}
	}
	return firstErr
}

// Sign отправляет запрос на подпись одной накладной.
func (sc *Scraper) Sign(number string) {
	select {
	case sc.signCh <- number:
	default:
		log.Printf(">>> signCh переполнен, пропускаю %s", number)
	}
}

// SignAll подписывает все текущие накладные.
func (sc *Scraper) SignAll() <-chan error {
	resp := make(chan error, 1)
	select {
	case sc.signAllCh <- resp:
	default:
		log.Printf(">>> signAllCh переполнен, пропускаю запрос")
	}
	return resp
}

// GetListNotes асинхронно получает список накладных.
func (sc *Scraper) GetListNotes() <-chan []DeliveryNote {
	resp := make(chan []DeliveryNote, 1)
	select {
	case sc.listCh <- resp:
	default:
		log.Printf(">>> listCh переполнен, пропускаю запрос")
	}
	return resp
}

func (sc *Scraper) initSession(ctx context.Context) error {
	if err := NavigateToLogin(ctx); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}
	dismissCookieBanner(ctx)

	if ElementExists(ctx, "Сертификат") {
		if err := Login(ctx, sc.session.cfg.CertUser); err != nil {
			return fmt.Errorf("login: %w", err)
		}
	}

	if err := NavigateToCarrier(ctx); err != nil {
		return fmt.Errorf("carrier: %w", err)
	}
	return nil
}

func closePopups(ctx context.Context) {
	for i := 0; i < 3; i++ {
		chromedp.Run(ctx, chromedp.KeyEvent("\x1b"))
		chromedp.Run(ctx, chromedp.Sleep(500*time.Millisecond))
	}
	chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
}

func reloadNoCache(ctx context.Context) {
	chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCacheDisabled(true).Do(ctx)
	}))
	chromedp.Run(ctx, chromedp.Reload())
	chromedp.Run(ctx, chromedp.Sleep(3*time.Second))
	chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCacheDisabled(false).Do(ctx)
	}))
}

func (sc *Scraper) fetchNotes(ctx context.Context) ([]DeliveryNote, error) {
	notes, err := ParseDeliveryNotes(ctx)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	var filtered []DeliveryNote
	for _, n := range notes {
		if n.Number != "" {
			filtered = append(filtered, n)
		}
	}
	sc.session.mu.Lock()
	sc.session.notes = filtered
	sc.session.mu.Unlock()
	return filtered, nil
}
