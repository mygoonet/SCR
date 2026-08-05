package main

import (
	"SCR/SCRP"
	"fmt"
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

	Layout := ("2006-01-02 15:04:05")
	t := fmt.Sprintf("%s", time.Now().Format(Layout))
	go telegram.Sendf("scraper in started - %s", t)
	// Монитор — живёт всегда, опрос каждые 10 минут.
	// recover ловит панику из chromedp/ плагина, чтобы процесс не падал
	// с кодом 2 без следа в логах.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("!!! Monitor panic (recovered): %v", r)
			}
		}()
		SCRP.Monitor(browser, cfg, telegram, 10*time.Minute)
	}()

	// Для Telegram-бота (вызывается из другого кода):
	// err := SCRP.SignSession(browser, cfg, []string{"000008514"})
	// if err != nil { log.Printf("ошибка: %v", err) }

	select {}
}
