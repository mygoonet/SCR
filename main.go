package main

import (
	"SCR/SCRP"
	"fmt"
	"log"
	"time"
)

func main() {
	loc, err := time.LoadLocation("Asia/Irkutsk")
	if err != nil {
		log.Fatalf("load location: %v", err)
	}
	time.Local = loc

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

	go SCRP.StartWebServer(cfg.WebAddr)
	go SCRP.StartScreenshotsCleanup()

	cmdCh := make(chan SCRP.MonitorCmd, 2)

	// TEST: управление монитором через канал (до запуска — канал буферизован).

	Layout := "2006-01-02 15:04:05"
	t := fmt.Sprintf("%s", time.Now().Format(Layout))

	go telegram.Sendf("scraper in started - %s", t)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("!!! Monitor panic (recovered): %v", r)
			}
		}()
		SCRP.MonitorAPI(browser, cfg, telegram, cmdCh)

		// Для автономного теста логин + запрос к API:
		//SCRP.TestAPI(browser, cfg)
	}()

	go func() {

		/*	time.Sleep(1 * time.Minute)
			cmdCh <- SCRP.MonitorCmd{AutoSign: boolPtr(true)}
			time.Sleep(3 * time.Minute)
			cmdCh <- SCRP.MonitorCmd{AutoSign: boolPtr(false)}*/
	}()

	// Для Telegram-бота (вызывается из другого кода):
	// err := SCRP.SignSession(browser, cfg, []string{"000008514"})
	// if err != nil { log.Printf("ошибка: %v", err) }

	select {}
}

func boolPtr(b bool) *bool { return &b }
