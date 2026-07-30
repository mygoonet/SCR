package SCRP

import (
	"encoding/json"
	"log"
	"os"

	"github.com/chromedp/chromedp"
)

func Run() {

	target := "000008515" // номер накладной для подписания — меняй здесь

	s := New(ConfigFromEnv())

	if err := s.Open(); err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	ctx := s.Ctx()

	// Шаг 1: перейти на страницу логина
	if err := NavigateToLogin(ctx); err != nil {
		log.Fatal(err)
	}

	// Шаг 2: принять куки если есть
	DismissCookieBanner(ctx)

	// Шаг 3: два варианта логина
	loggedIn, err := LoginIfNeeded(ctx, s.Cfg().CertUser)
	if err != nil {
		log.Fatal("login:", err)
	}
	if loggedIn {
		log.Println("Выполнен вход через сертификат")
	} else {
		log.Println("Уже залогинены, пропускаем авторизацию")
	}

	// Скриншот после входа
	var buf []byte
	if err := chromedp.Run(ctx, chromedp.FullScreenshot(&buf, 90)); err == nil {
		os.WriteFile("/tmp/opencode/after_login.png", buf, 0644)
		log.Println("Скриншот после входа сохранён: /tmp/opencode/after_login.png")
	}

	var currentURL string
	chromedp.Run(ctx, chromedp.Location(&currentURL))
	log.Printf("Текущий URL после входа: %s", currentURL)

	// Шаг 4: перейти в раздел Перевозчика
	if err := NavigateToCarrier(ctx); err != nil {
		log.Fatal("carrier:", err)
	}

	// Скриншот после выбора перевозчика
	var buf2 []byte
	if err := chromedp.Run(ctx, chromedp.FullScreenshot(&buf2, 90)); err == nil {
		os.WriteFile("/tmp/opencode/after_carrier.png", buf2, 0644)
		log.Println("Скриншот после выбора перевозчика: /tmp/opencode/after_carrier.png")
	}

	// Шаг 5: получить список накладных
	notes, err := ParseDeliveryNotes(ctx)
	if err != nil {
		log.Fatal("parse:", err)
	}
	b, _ := json.MarshalIndent(notes, "", "  ")
	log.Printf("Получено %d накладных:\n%s", len(notes), string(b))

	// Шаг 6: подписать накладную
	log.Printf("Пробуем подписать накладную %s...", target)
	if err := SignDeliveryNote(ctx, target, s.Cfg().CertUser); err != nil {
		log.Fatal("sign:", err)
	}
	log.Println("Подписание завершено успешно!")
}
