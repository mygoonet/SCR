package SCRP

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

type MonitorCmd struct {
	Interval time.Duration // <=0 = не менять
	AutoSign *bool         // nil = не менять
}

func Monitor(browser *Browser, cfg Config, tel *TelegramClient, cmdCh <-chan MonitorCmd) {
	session := browser.NewSession()
	defer session.Close()

	ctx := session.Ctx()

	if err := initSession(ctx, cfg); err != nil {
		log.Printf("Monitor: init session: %v", err)
		return
	}

	interval := 10 * time.Minute
	autoSign := true

	log.Println("Monitor запущен. Тикер:", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	resetCh := make(chan struct{}, 1)
	stopAnnouncer := startAnnouncer(ctx, interval, resetCh)
	resetCh <- struct{}{}

	for {
		select {
		case cmd, ok := <-cmdCh:
			if !ok {
				log.Println("Monitor: канал управления закрыт, завершаюсь")
				return
			}
			if cmd.Interval > 0 {
				interval = cmd.Interval
				ticker.Reset(interval)
				stopAnnouncer()
				stopAnnouncer = startAnnouncer(ctx, interval, resetCh)
				resetCh <- struct{}{}
			}
			if cmd.AutoSign != nil {
				autoSign = *cmd.AutoSign
			}
			log.Printf("Monitor: параметры обновлены (interval=%s, autosign=%v)", interval, autoSign)
		case <-ticker.C:
			resetCh <- struct{}{}

			notes, err := fetchNotes(ctx)
			if err != nil {
				log.Printf("Monitor: ошибка получения накладных: %v", err)
				continue
			}

			log.Printf("Monitor: накладные (%d):", len(notes))
			for _, n := range notes {
				log.Printf("  %s  от %s  %s → %s  %s", n.Number, n.Date, n.Consignor, n.Consignee, n.Carrier)
			}

			todo := signableNotes(notes)
			if len(todo) > 0 {
				if tel != nil {
					nums := make([]string, 0, len(todo))
					for _, n := range todo {
						nums = append(nums, n.Number)
					}
					tel.Sendf("ВНИМАНИЕ: накладные на подпись (%d): %v", len(todo), nums)
				}
			}

			if autoSign && len(todo) > 0 {
				if err := signAll(ctx, cfg.CertUser, todo, tel); err != nil {
					log.Printf("Monitor: SignAll ошибка: %v", err)
				} else {
					log.Println("Monitor: SignAll завершено")
					if tel != nil {
						tel.Sendf("Monitor: SignAll завершено")
					}
				}
			}
		}
	}
}

func signableNotes(notes []DeliveryNote) []DeliveryNote {
	var out []DeliveryNote
	for _, n := range notes {
		if n.Number != "" && !skipNumbers[n.Number] {
			out = append(out, n)
		}
	}
	return out
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

	return signAll(ctx, cfg.CertUser, notes, nil)
}

func initSession(ctx context.Context, cfg Config) error {
	// Кэш отключаем один раз на всю сессию и больше не включаем.
	chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCacheDisabled(true).Do(ctx)
	}))
	takeScreenshot(ctx, "start_init_cert")
	if err := NavigateToLogin(ctx); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}

	DismissCookieBanner(ctx)

	if ElementExists(ctx, "Сертификат") {
		log.Println("initSession: found Сертификат, логинюсь")
		if err := Login(ctx, cfg.CertUser); err != nil {
			return fmt.Errorf("login: %w", err)
		}
		log.Println("initSession: login OK")
	} else {
		log.Println("initSession: already logged in")
	}

	if err := NavigateToCarrier(ctx); err != nil {
		return fmt.Errorf("carrier: %w", err)
	}
	log.Println("initSession: carrier OK")
	return nil
}

var skipNumbers = map[string]bool{

	"000010271": true,
	"000010270": true,
	"000010269": true,

	//"000000420": true, //<--- dont remove//
}

func signAll(ctx context.Context, certUser string, notes []DeliveryNote, tel *TelegramClient) error {
	var firstErr error
	for _, n := range notes {
		if skipNumbers[n.Number] {
			log.Printf(">>> signAll: пропускаю %s (тестовая)", n.Number)
			continue
		}
		signed := false
		for attempt := 1; attempt <= 3; attempt++ {
			log.Printf(">>> signAll: подписываю %s (попытка %d)...", n.Number, attempt)
			if tel != nil {
				tel.Sendf(">>> signAll: подписываю %s (попытка %d)...", n.Number, attempt)
			}

			// Жёсткий таймаут на всю попытку подписания — если
			// DevTools-сессия зависнет (нативный диалог криптоплагина),
			// цикл подписания не встанет навсегда.
			signCtx, signCancel := context.WithTimeout(ctx, 3*time.Minute)
			err := SignDeliveryNote(signCtx, n.Number, certUser)
			signCancel()
			if err != nil {
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
			if tel != nil {
				tel.Sendf(">>> Накладная %s подписана, жду обновления страницы...", n.Number)
			}
			closePopups(ctx)
			// Серверный список обновляется с лагом — опрашиваем до 4 раз
			// (по ~8с каждый), прежде чем решить что накладная ещё в списке.
			signed = true
			for check := 0; check < 4; check++ {
				reloadNoCache(ctx)
				waitForTableRows(ctx)
				refreshed, err := fetchNotes(ctx)
				if err != nil {
					log.Printf(">>> Ошибка обновления списка: %v", err)
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
				log.Printf(">>> Накладная %s ещё в списке (проверка %d/4), жду...", n.Number, check+1)
				chromedp.Run(ctx, chromedp.Sleep(10*time.Second))
				signed = false
			}
			if signed {
				break
			}
			log.Printf(">>> Накладная %s не пропала после 4 проверок, повторяю подписание...", n.Number)
		}
		if !signed {
			log.Printf(">>> Накладная %s не подписана после 3 попыток", n.Number)
		} else {
			// Сбрасываем ошибку — по факту всё подписано.
			firstErr = nil
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
	// Кэш уже отключён навсегда в initSession и chrome flags,
	// здесь просто перезагружаем страницу — кэш не включаем обратно.
	chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCacheDisabled(true).Do(ctx)
	}))
	chromedp.Run(ctx, chromedp.Reload())
	chromedp.Run(ctx, chromedp.Sleep(3*time.Second))
}

func startAnnouncer(ctx context.Context, interval time.Duration, resetCh chan struct{}) (stop func()) {
	stopCh := make(chan struct{})
	var once sync.Once
	stop = func() {
		once.Do(func() { close(stopCh) })
	}
	go countdownAnnouncer(ctx, interval, resetCh, stopCh)
	return stop
}

func countdownAnnouncer(ctx context.Context, interval time.Duration, resetCh chan struct{}, stopCh chan struct{}) {
	minutes := int(interval.Minutes())
	for {
		for m := minutes; m > 0; m-- {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
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
