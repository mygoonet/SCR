package SCRP

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func Monitor(browser *Browser, cfg Config, interval time.Duration) {
	session := browser.NewSession()
	defer session.Close()

	ctx := session.Ctx()

	if err := initSession(ctx, cfg); err != nil {
		log.Printf("Monitor: init session: %v", err)
		return
	}

	log.Println("Monitor запущен. Тикер:", interval)

	notes, err := fetchNotes(ctx)
	if err != nil {
		log.Printf("Monitor: ошибка получения накладных: %v", err)
	} else {
		log.Printf("Monitor: накладные (%d):", len(notes))
		for _, n := range notes {
			log.Printf("  %s  от %s  %s → %s  %s", n.Number, n.Date, n.Consignor, n.Consignee, n.Carrier)
		}
		if len(notes) > 0 {
			if err := signAll(ctx, cfg.CertUser, notes); err != nil {
				log.Printf("Monitor: SignAll ошибка: %v", err)
			} else {
				log.Println("Monitor: SignAll завершено")
			}
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	resetCh := make(chan struct{}, 1)
	go countdownAnnouncer(ctx, interval, resetCh)
	resetCh <- struct{}{}

	for range ticker.C {
		resetCh <- struct{}{}

		reloadNoCache(ctx)

		waitForTableRows(ctx)

		notes, err := fetchNotes(ctx)
		if err != nil {
			log.Printf("Monitor: ошибка получения накладных: %v", err)
			continue
		}

		log.Printf("Monitor: накладные (%d):", len(notes))
		for _, n := range notes {
			log.Printf("  %s  от %s  %s → %s  %s", n.Number, n.Date, n.Consignor, n.Consignee, n.Carrier)
		}

		if len(notes) > 0 {
			if err := signAll(ctx, cfg.CertUser, notes); err != nil {
				log.Printf("Monitor: SignAll ошибка: %v", err)
			} else {
				log.Println("Monitor: SignAll завершено")
			}
		}
	}
}

func SignSession(browser *Browser, cfg Config, numbers []string) error {
	session := browser.NewSession()
	defer session.Close()

	ctx := session.Ctx()

	if err := initSession(ctx, cfg); err != nil {
		return fmt.Errorf("init session: %w", err)
	}

	var notes []DeliveryNote
	for _, num := range numbers {
		notes = append(notes, DeliveryNote{Number: num})
	}

	return signAll(ctx, cfg.CertUser, notes)
}

func initSession(ctx context.Context, cfg Config) error {
	//CACHE IS DISABLED GLOBAL
	chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCacheDisabled(true).Do(ctx)
	}))

	if err := NavigateToLogin(ctx); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}
	DismissCookieBanner(ctx)

	if ElementExists(ctx, "Сертификат") {
		if err := Login(ctx, cfg.CertUser); err != nil {
			return fmt.Errorf("login: %w", err)
		}
	}

	if err := NavigateToCarrier(ctx); err != nil {
		return fmt.Errorf("carrier: %w", err)
	}
	return nil
}

var skipNumbers = map[string]bool{
	"000000420": true,
}

func signAll(ctx context.Context, certUser string, notes []DeliveryNote) error {
	var firstErr error
	for _, n := range notes {
		if skipNumbers[n.Number] {
			log.Printf(">>> signAll: пропускаю %s (тестовая)", n.Number)
			continue
		}
		signed := false
		for attempt := 1; attempt <= 3; attempt++ {
			log.Printf(">>> signAll: подписываю %s (попытка %d)...", n.Number, attempt)
			if err := SignDeliveryNote(ctx, n.Number, certUser); err != nil {
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
			refreshed, err := fetchNotes(ctx)
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

func countdownAnnouncer(ctx context.Context, interval time.Duration, resetCh chan struct{}) {
	minutes := int(interval.Minutes())
	for {
		for m := minutes; m > 0; m-- {
			select {
			case <-ctx.Done():
				return
			case <-resetCh:
				log.Println("⏳ Обратный отсчёт перезапущен")
				m = minutes + 1
			case <-time.After(time.Minute):
				log.Printf("⏳ До следующего обновления: %d мин", m-1)
			}
		}
	}
}

func fetchNotes(ctx context.Context) ([]DeliveryNote, error) {
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
	return filtered, nil
}
