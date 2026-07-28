package main

import (
	"encoding/json"
	"log"

	"SCR/SCRP"
)

func main() {
	target := "000008517"

	s := SCRP.New(SCRP.Config{
		UserDataDir: "/home/visa/.config/chromium-gost-scrp",
		ChromePath:  "/usr/bin/chromium-gost-stable",
		CertUser:    "Сичкарук Евгений Александрович",
	})

	if err := s.Open(); err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	ctx := s.Ctx()

	// Шаг 1: перейти на страницу логина
	if err := SCRP.NavigateToLogin(ctx); err != nil {
		log.Fatal(err)
	}

	// Шаг 2: принять куки если есть
	SCRP.DismissCookieBanner(ctx)

	// Шаг 3: два варианта логина
	loggedIn, err := SCRP.LoginIfNeeded(ctx, s.Cfg().CertUser)
	if err != nil {
		log.Fatal("login:", err)
	}
	if loggedIn {
		log.Println("Выполнен вход через сертификат")
	} else {
		log.Println("Уже залогинены, пропускаем авторизацию")
	}

	// Шаг 4: перейти в раздел Перевозчика
	if err := SCRP.NavigateToCarrier(ctx); err != nil {
		log.Fatal("carrier:", err)
	}

	// Шаг 5: получить список накладных
	notes, err := SCRP.ParseDeliveryNotes(ctx)
	if err != nil {
		log.Fatal("parse:", err)
	}
	b, _ := json.MarshalIndent(notes, "", "  ")
	log.Printf("Получено %d накладных:\n%s", len(notes), string(b))

	// Шаг 6: подписать накладную
	log.Printf("Пробуем подписать накладную %s...", target)
	if err := SCRP.SignDeliveryNote(ctx, target); err != nil {
		log.Fatal("sign:", err)
	}
	log.Println("Подписание завершено успешно!")
}
