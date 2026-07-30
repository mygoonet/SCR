package main

import (
	"SCR/SCRP"
	"log"
)

func main() {
	cfg := SCRP.Config{
		UserDataDir: "/home/visa/.config/chromium-gost-scrp",
		ChromePath:  "/usr/bin/chromium-gost-stable",
		CertUser:    "Сичкарук Евгений Александрович",
	}

	scraper := SCRP.NewScraper(cfg)

	// Запускаем скрепер в горутине
	go func() {
		if err := scraper.Start(); err != nil {
			log.Fatalf("scraper: %v", err)
		}
	}()

	// Команды: подпись + запрос списка
	/*go func() {
		time.Sleep(30 * time.Second)

		log.Println(">>> Отправка на подпись: 000008514")
		scraper.Sign("000008514")

		time.Sleep(10 * time.Second)

		log.Println(">>> Отправка на подпись: 000008515")
		scraper.Sign("000008515")

		time.Sleep(5 * time.Second)

		log.Println(">>> Запрос списка накладных")
		listCh := scraper.GetListNotes()
		notes := <-listCh
		if notes == nil {
			log.Println(">>> Список не получен")
		} else {
			log.Printf(">>> Получено %d накладных", len(notes))
			for _, n := range notes {
				log.Printf("  %s  от %s", n.Number, n.Date)
			}
		}

		scraper.SignAll()

	}() */

	// Даем время на обработку
	select {}
}
