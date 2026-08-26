package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"SCR/SCRP"
)

func main() {
	out := flag.String("out", "", "путь для сырого JSON (по умолчанию /home/visa/SCRP/tmp/transportations-<ts>.json)")
	timeoutSec := flag.Int("timeout", 40, "секунд на перехват запроса /transportations")
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
		cfg.UserDataDir = "/home/visa/.config/chromium-gost-scrp"
	}

	if *out == "" {
		*out = fmt.Sprintf("/home/visa/SCRP/tmp/transportations-%s.json", time.Now().Format("20060102-150405"))
	}

	browser := SCRP.NewBrowser(cfg)
	defer browser.Close()

	path, err := SCRP.DumpTransportationsRaw(browser, cfg, *out, *timeoutSec)
	if err != nil {
		log.Fatalf("dump: %v", err)
	}
	fmt.Println("RAW_JSON_FILE:", path)
}
