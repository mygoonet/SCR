package main

import (
	"encoding/json"
	"log"

	"SCR/SCRP"
)

func main() {
	target := "000008517"

	s := SCRP.New(SCRP.Config{
		UserDataDir: "/home/visa/.config/google-chrome",
		ChromePath:  "/usr/bin/chromium-gost-stable",
		CertUser:    "Сичкарук Евгений Александрович",
	})

	if err := s.Open(); err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	if err := s.Run(); err != nil {
		log.Fatal("first run:", err)
	}

	notes := s.DeliveryNotes()
	b, _ := json.MarshalIndent(notes, "", "  ")
	log.Printf("Получено %d накладных:\n%s", len(notes), string(b))

	log.Printf("Пробуем подписать накладную %s...", target)
	if err := SCRP.SignDeliveryNote(s.Ctx(), target); err != nil {
		log.Fatal("sign:", err)
	}
	log.Println("Подписание завершено успешно!")
}
