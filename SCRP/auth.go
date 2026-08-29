package SCRP

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/chromedp"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func NavigateToLogin_old(ctx context.Context) error {
	if err := chromedp.Run(ctx,
		chromedp.Navigate("https://logist.kontur.ru/box-selection"),
		chromedp.WaitReady(`body`),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return err
	}

	// Ждём до 10 секунд, проверяя каждые 0.5с
	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for page content")
		case <-ticker.C:
			var textLen int
			if err := chromedp.Run(ctx, chromedp.Evaluate(`document.body.innerText.length`, &textLen)); err != nil {
				return fmt.Errorf("evaluate text length: %w", err)
			}
			if textLen > 200 {
				return nil
			}
		}
	}
}

func NavigateToLogin(ctx context.Context) error {

	if err := chromedp.Run(ctx,
		chromedp.Navigate("https://logist.kontur.ru/box-selection"),

		chromedp.WaitReady(`body`),

		chromedp.Sleep(5*time.Second),
	); err != nil {
		return err
	}

	//takeScreenshot(ctx, "01_start")
	for i := 0; i < 20; i++ {
		var textLen int
		chromedp.Run(ctx, chromedp.Evaluate(`document.body.innerText.length`, &textLen))
		if textLen > 200 {
			break
		}
		err := chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
		if err != nil {
			return err
		}
	}

	return nil
}

func Login(ctx context.Context, certUser string) error {
	DismissCookieBanner(ctx)

	takeScreenshot(ctx, "login_auth_page")

	if err := ReactClick(ctx, "Сертификат"); err != nil {
		return fmt.Errorf("click Сертификат: %w", err)
	}

	// Список сертификатов грузится не сразу — ждём появления нужного (до 30с).
	deadline := time.Now().Add(30 * time.Second)
	for !ElementExistsContains(ctx, certUser) {
		if time.Now().After(deadline) {
			log.Printf("Login: page text: %q", PageText(ctx)[:min(800, len(PageText(ctx)))])
			return fmt.Errorf("wait %s: timeout", certUser)
		}
		time.Sleep(1 * time.Second)
	}
	chromedp.Run(ctx, chromedp.Sleep(1*time.Second))

	takeScreenshot(ctx, "login_after_cert_click")

	if err := ReactClickContains(ctx, certUser); err != nil {
		return fmt.Errorf("click %s: %w", certUser, err)
	}

	return chromedp.Run(ctx,
		chromedp.WaitReady(`body`),
		chromedp.Sleep(5*time.Second),
	)
}
