package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"SCR/SCRP"
)

func main() {
	flag := flag.NewFlagSet("scrpcheck", flag.ExitOnError)
	signNum := flag.String("sign", "", "номер накладной для полного прогона подписания (после проверки разметки)")
	fresh := flag.Bool("fresh", false, "сбросить сессию перед проверкой — пройти ПОЛНЫЙ путь логина (а не по сохранённой сессии)")
	flag.Parse(os.Args[1:])

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

	// При -fresh НЕ трогаем рабочий профиль: копируем его во временную
	// папку и удаляем ТОЛЬКО файлы сессии (Cookies + Local Storage) уже в
	// копии — этого достаточно, чтобы Контур показал окно входа и initSession
	// прошёл ПОЛНЫЙ путь логина. Состояние криптоплагина (другие файлы профиля)
	// сохраняется. Рабочий монитор не пострадает.
	if *fresh {
		tmp, err := os.MkdirTemp("", "scrp-fresh-")
		if err != nil {
			log.Fatalf("mkdir temp: %v", err)
		}
		defer os.RemoveAll(tmp)
		if err := copyDir(cfg.UserDataDir, tmp); err != nil {
			log.Fatalf("copy profile %s -> %s: %v", cfg.UserDataDir, tmp, err)
		}
		purgeSessionFiles(tmp)
		log.Printf("scrpcheck: -fresh использует КОПИЮ профиля %s (сессия сброшена)", tmp)
		cfg.UserDataDir = tmp
	}

	browser := SCRP.NewBrowser(cfg)
	defer browser.Close()

	rep := SCRP.CheckSite(browser, cfg, *fresh)

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

// purgeSessionFiles удаляет только файлы сессии (Cookies + Local Storage)
// в профиле chromium — этого достаточно, чтобы Контур показал окно входа.
// Состояние криптоплагина (другие файлы профиля) НЕ трогаем. Вызывать ТОЛЬКО
// на КОПИИ профиля (см. -fresh), никогда на рабочем.
func purgeSessionFiles(profileDir string) {
	patterns := []string{
		"*Cookies*",
		"*Cookies",
		"*Local Storage*",
		"Local Storage",
		"*Session Storage*",
		"*Web Data*",
	}
	var removed int
	filepath.Walk(profileDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := info.Name()
		for _, p := range patterns {
			if matched, _ := filepath.Match(p, name); matched {
				if e := os.Remove(path); e == nil {
					removed++
				}
				break
			}
		}
		return nil
	})
	log.Printf("purgeSessionFiles: удалено файлов сессии = %d", removed)
}

// copyDir рекурсивно копирует профиль chromium (файлы + симлинки) во временную
// папку. Используется -fresh, чтобы сбрасывать сессию на КОПИИ профиля и не
// трогать рабочий (живой монитор в контейнере).
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, e := os.Readlink(path)
			if e != nil {
				return e
			}
			return os.Symlink(link, target)
		}
		in, e := os.Open(path)
		if e != nil {
			return e
		}
		defer in.Close()
		out, e := os.Create(target)
		if e != nil {
			return e
		}
		defer out.Close()
		_, e = io.Copy(out, in)
		return e
	})
}
