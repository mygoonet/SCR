package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"SCR/SCRP"
)

func main() {
	signNum := flag.String("sign", "", "номер накладной для полного прогона подписания (после проверки разметки)")
	flag.Parse()

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

	browser := SCRP.NewBrowser(cfg)
	defer browser.Close()

	rep := SCRP.CheckSite(browser, cfg)

	fmt.Println("================ SCRP CHECK ================")
	fmt.Printf("Login:        %v\n", rep.LoggedIn)
	fmt.Printf("OnCarrier:    %v\n", rep.OnCarrier)
	fmt.Printf("Notes count:  %d\n", rep.NotesCount)
	fmt.Println("--- Selectors ---")
	for _, s := range rep.Selectors {
		status := "OK "
		if !s.Found {
			status = "FAIL"
		}
		fmt.Printf("  [%s] %-35s count=%d\n", status, s.Name, s.Count)
	}
	fmt.Printf("Sign flow:    %v\n", rep.SignFlowOK)
	if rep.SignError != "" {
		fmt.Printf("  error: %s\n", rep.SignError)
	}
	fmt.Println("--------------------------------------------")

	if !rep.OK {
		fmt.Println("RESULT: FAIL — разметка/путь подписания изменились, нужен разбор")
		os.Exit(1)
	}

	fmt.Println("RESULT: OK — разметка не поменялась, путь подписания жив")

	// Полный прогон подписания на указанной накладной (например тестовой 000000420).
	if *signNum != "" {
		fmt.Printf("\n================ FULL SIGN: %s ================\n", *signNum)
		err := SCRP.FullSignDeliveryNote(browser, cfg, *signNum)
		if err != nil {
			fmt.Printf("FULL SIGN: FAIL — %v\n", err)
			os.Exit(1)
		}
		fmt.Println("FULL SIGN: OK — подписание прошло полностью")
	}
}
