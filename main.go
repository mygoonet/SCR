package main

import (
	"SCR/SCRP"
	"log"
	"time"
)

func main() {
	cfg := SCRP.ConfigFromEnv()

	if cfg.ChromePath == "" || cfg.CertUser == "" {
		log.Fatal("CHROME_PATH and CERT_USER env vars are required")
	}
	if cfg.UserDataDir == "" {
		cfg.UserDataDir = "/home/app/.config/chromium-gost-scrp"
	}

	telegram := SCRP.NewTelegramClient(cfg.TelegramToken, cfg.TelegramChat, cfg.SOCKS5)
	browser := SCRP.NewBrowser(cfg)
	defer browser.Close()

	go telegram.Sendf("TEST FROM SCRAPPER")
	// Монитор — живёт всегда, опрос каждые 10 минут
	go SCRP.Monitor(browser, cfg, telegram, 10*time.Minute)

	// Для Telegram-бота (вызывается из другого кода):
	// err := SCRP.SignSession(browser, cfg, []string{"000008514"})
	// if err != nil { log.Printf("ошибка: %v", err) }

	select {}
}
