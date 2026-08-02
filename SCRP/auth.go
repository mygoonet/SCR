package SCRP

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

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

	takeScreenshot(ctx, "LOGIN?CERTIFICATE")
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

	var buf []byte
	if err := chromedp.Run(ctx, chromedp.FullScreenshot(&buf, 90)); err == nil {
		os.WriteFile("/tmp/opencode/auth_page.png", buf, 0644)
	}

	if err := ClickElement(ctx, "Сертификат"); err != nil {
		return fmt.Errorf("click Сертификат: %w", err)
	}
	chromedp.Run(ctx, chromedp.Sleep(3*time.Second))

	if err := chromedp.Run(ctx,
		chromedp.WaitReady(`body`),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return fmt.Errorf("wait auth page: %w", err)
	}

	var buf2 []byte
	if err := chromedp.Run(ctx, chromedp.FullScreenshot(&buf2, 90)); err == nil {
		os.WriteFile("/tmp/opencode/after_cert_click.png", buf2, 0644)
	}

	if err := ClickElement(ctx, certUser); err != nil {
		return fmt.Errorf("click %s: %w", certUser, err)
	}

	return chromedp.Run(ctx,
		chromedp.WaitReady(`body`),
		chromedp.Sleep(5*time.Second),
	)
}
