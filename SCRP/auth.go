package SCRP

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
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

// authEvalTimeout — дедлайн ОДНОГО вызова Evaluate на странице входа. Без него
// зависший рендерер (плагины КриптоПро/Госуслуг) блокирует chromedp.Run
// навсегда: вся страница не отвечает, а у вызова нет таймаута.
const authEvalTimeout = 15 * time.Second

// evalBool — Evaluate c дедлайном на один вызов.
func evalBool(ctx context.Context, js string) (bool, error) {
	c, cancel := context.WithTimeout(ctx, authEvalTimeout)
	defer cancel()
	var ok bool
	if err := chromedp.Run(c, chromedp.Evaluate(js, &ok)); err != nil {
		return false, err
	}
	return ok, nil
}

// evalStr — Evaluate (строка) c дедлайном на один вызов.
func evalStr(ctx context.Context, js string) (string, error) {
	c, cancel := context.WithTimeout(ctx, authEvalTimeout)
	defer cancel()
	var s string
	if err := chromedp.Run(c, chromedp.Evaluate(js, &s)); err != nil {
		return "", err
	}
	return s, nil
}

// waitContainsText — ждёт (до timeout) появления элемента, ЧЕЙ ТЕКСТ содержит
// text. Каждый Evaluate ограничен authEvalTimeout, поэтому неответившая
// страница даёт ошибку, а не вечное зависание.
func waitContainsText(ctx context.Context, text string, timeout time.Duration) error {
	js := fmt.Sprintf(`(function(t){
		var all = document.querySelectorAll('*');
		for(var i=0;i<all.length;i++){
			if(all[i].textContent.replace(/\u00a0/g,' ').includes(t)) return true;
		}
		return false;
	})(%q)`, text)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ok, err := evalBool(ctx, js)
		if err != nil {
			return fmt.Errorf("page not responding: %w", err)
		}
		if ok {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout %s", timeout)
}

// authShot — быстрый скриншот ВИДИМОЙ области с дедлайном. chromedp.FullScreenshot
// (captureBeyondViewport) на странице входа Контура рисует 40–65 с — именно он
// превращал логин в «вечное зависание». Только для диагностических шагов
// логина, НЕ для шагов подписания (те пишутся в папку накладной через takeScreenshot).
func authShot(ctx context.Context, name string) time.Duration {
	t0 := time.Now()
	var buf []byte
	c, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := chromedp.Run(c, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		buf, err = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatPng).Do(ctx)
		return err
	})); err != nil {
		log.Printf("screenshot %s: %v", name, err)
		return time.Since(t0)
	}
	captureFlowShot(name, buf)
	path := filepath.Join(screenshotDir, name+".png")
	if err := os.WriteFile(path, buf, 0644); err != nil {
		log.Printf("save screenshot %s: %v", path, err)
	}
	return time.Since(t0)
}

func Login(ctx context.Context, certUser string) error {
	t0 := time.Now()
	step := func(what string) { log.Printf("Login: %s (+%s)", what, time.Since(t0).Round(time.Millisecond)) }

	DismissCookieBanner(ctx)
	step("cookie banner dismissed")

	log.Printf("Login: shot +%s", authShot(ctx, "login_auth_page").Round(time.Millisecond))

	// Список сертификатов Контур показывает и без клика по табу «Сертификат»
	// (он активен по умолчанию). Если карточка уже есть — таб не трогаем:
	// клик по уже активному табу только сбрасывает список.
	cardPresent, err := evalBool(ctx, `document.querySelector('[data-tid^="certificate_"]') !== null`)
	if err != nil {
		return fmt.Errorf("check cert card: %w", err)
	}
	if cardPresent {
		log.Println("Login: карточка сертификата уже видна, таб «Сертификат» не кликаю")
	} else if err := ReactClick(ctx, "Сертификат"); err != nil {
		return fmt.Errorf("click Сертификат: %w", err)
	}
	step("cert card checked")

	// Список сертификатов грузится не сразу — ждём появления нужного (до 30с).
	if err := waitContainsText(ctx, certUser, 30*time.Second); err != nil {
		step("name wait failed")
		pt, perr := evalStr(ctx, `document.body.innerText`)
		if perr != nil {
			log.Printf("Login: page text недоступен: %v", perr)
		} else {
			log.Printf("Login: page text: %q", pt[:min(800, len(pt))])
		}
		return fmt.Errorf("wait %s: %w", certUser, err)
	}
	chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
	step("sleep after name")

	log.Printf("Login: shot +%s", authShot(ctx, "login_after_cert_click").Round(time.Millisecond))

	if err := ReactClickContains(ctx, certUser); err != nil {
		return fmt.Errorf("click %s: %w", certUser, err)
	}
	step("clicked name")

	// Диагностика: структура карточки сертификата сразу после клика.
	{
		dctx, dcancel := context.WithTimeout(ctx, authEvalTimeout)
		defer dcancel()
		log.Printf("Login: certCardDump=%s", DumpCertCard(dctx, certUser))
	}
	step("cert card dumped")

	// После выбора сертификата/пользователя Контур делает редирект через
	// auth.kontur.ru / identity.kontur.ru обратно в logist.kontur.ru. Ждём
	// завершения входа: URL должен уйти с auth/identity и вернуться на
	// logist.kontur.ru. Без этого NavigateToCarrier застаёт страницу на
	// auth-редиректе и вечно ждёт кнопку "Перевозчик" (которой там нет).
	var finalURL string
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		var url string
		c, cancel := context.WithTimeout(ctx, authEvalTimeout)
		lerr := chromedp.Run(c, chromedp.Location(&url))
		cancel()
		if lerr != nil {
			return fmt.Errorf("read url after login: %w", lerr)
		}
		finalURL = url
		if strings.Contains(url, "logist.kontur.ru") && !strings.Contains(url, "auth.kontur.ru") && !strings.Contains(url, "identity.kontur.ru") {
			break
		}
		sc, scancel := context.WithTimeout(ctx, authEvalTimeout)
		serr := chromedp.Run(sc, chromedp.Sleep(1*time.Second))
		scancel()
		if serr != nil {
			return fmt.Errorf("wait redirect: %w", serr)
		}
	}
	log.Printf("Login: после входа url=%s", finalURL)

	// Финальная выдержка на url=logist: без дедлайна WaitReady может зависнуть
	// на навигации, и Login не вернётся никогда.
	fctx, fcancel := context.WithTimeout(ctx, 20*time.Second)
	defer fcancel()
	if err := chromedp.Run(fctx,
		chromedp.WaitReady(`body`),
		chromedp.Sleep(2*time.Second),
	); err != nil {
		return fmt.Errorf("wait page after login: %w", err)
	}
	return nil
}
