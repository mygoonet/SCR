package main

import (
	"encoding/json"
	"log"
	"time"

	"SCR/SCRP"
)

func main() {
	s := SCRP.New(SCRP.Config{
		UserDataDir: "/home/visa/.config/google-chrome",
		ChromePath:  "/usr/bin/chromium-gost-stable",
		CertUser:    "Сичкарук Евгений Александрович",
	})

	if err := s.Open(); err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		if err := s.Run(); err != nil {
			log.Println(err)
			<-ticker.C
			continue
		}

		notes := s.DeliveryNotes()
		b, _ := json.MarshalIndent(notes, "", "  ")
		log.Printf("Получено %d накладных:\n%s", len(notes), string(b))

		<-ticker.C
	}
}
