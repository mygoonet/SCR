package SCRP

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

func NavigateToLogin(ctx context.Context) error {
	if err := chromedp.Run(ctx,
		chromedp.Navigate("https://logist.kontur.ru/box-selection"),
		chromedp.WaitReady(`body`),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return err
	}

	for i := 0; i < 20; i++ {
		var textLen int
		chromedp.Run(ctx, chromedp.Evaluate(`document.body.innerText.length`, &textLen))
		if textLen > 200 {
			break
		}
		chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
	}

	return nil
}

func Login(ctx context.Context, certUser string) error {
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

	if err := ClickElement(ctx, certUser); err != nil {
		return fmt.Errorf("click %s: %w", certUser, err)
	}

	return chromedp.Run(ctx,
		chromedp.WaitReady(`body`),
		chromedp.Sleep(5*time.Second),
	)
}
