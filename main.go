package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
)

var userDataDir = "/home/visa/.config/google-chrome"

func cleanStaleLock() {
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		p := filepath.Join(userDataDir, name)
		os.Remove(p)
	}
}

func NewBrowser() (context.Context, context.CancelFunc) {
	cleanStaleLock()

	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath("/usr/bin/chromium-gost-stable"),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Headless,
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-popup-blocking", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("enable-automation", true),
		chromedp.Flag("password-store", "basic"),
		chromedp.Flag("use-mock-keychain", true),
		chromedp.Flag("disable-features", "Translate"),
		chromedp.Flag("user-data-dir", userDataDir),
	}

	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	return ctx, cancel
}

func clickElement(ctx context.Context, text string) bool {
	var result string
	err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(
		`(function(t){
		var all = document.querySelectorAll('*');
		for(var i=0;i<all.length;i++){
			if(all[i].textContent.trim() === t){
				var el = all[i];
				while(el && el.tagName !== 'A' && el.tagName !== 'BUTTON' && !el.onclick){
					el = el.parentElement;
				}
				if(el) el.click();
				return 'clicked ' + el.tagName;
			}
		}
		return 'not found';
	})(%q)`, text), &result))
	if err != nil {
		log.Printf("clickElement(%s) error: %v", text, err)
		return false
	}
	fmt.Printf("clickElement(%s): %s\n", text, result)
	return result != "not found"
}

func elementExists(ctx context.Context, text string) bool {
	var exists bool
	chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(
		`(function(t){
		var all = document.querySelectorAll('*');
		for(var i=0;i<all.length;i++){
			if(all[i].textContent.trim() === t) return true;
		}
		return false;
	})(%q)`, text), &exists))
	return exists
}

func doLogin(ctx context.Context) error {
	fmt.Println("--- Login: Click Сертификат ---")
	if !clickElement(ctx, "Сертификат") {
		return fmt.Errorf("Сертификат not found")
	}
	chromedp.Run(ctx, chromedp.Sleep(3*time.Second))

	fmt.Println("--- Login: Wait for auth page ---")
	if err := chromedp.Run(ctx, chromedp.WaitReady(`body`), chromedp.Sleep(5*time.Second)); err != nil {
		return err
	}

	var authBody string
	_ = chromedp.Run(ctx, chromedp.Evaluate(`document.body.innerText`, &authBody))
	fmt.Printf("Auth page body:\n%s\n", authBody)

	fmt.Println("--- Login: Click Сичкарук ---")
	if !clickElement(ctx, "Сичкарук Евгений Александрович") {
		return fmt.Errorf("Сичкарук not found")
	}

	fmt.Println("--- Login: Wait for redirect ---")
	return chromedp.Run(ctx,
		chromedp.WaitReady(`body`),
		chromedp.Sleep(5*time.Second),
	)
}

type DeliveryNote struct {
	Number      string `json:"number"`
	Consignor   string `json:"consignor"`
	Consignee   string `json:"consignee"`
	Address     string `json:"address"`
	Cargo       string `json:"cargo"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

func parseDeliveryNotes(ctx context.Context) ([]DeliveryNote, error) {
	var data []DeliveryNote
	err := chromedp.Run(ctx, chromedp.Evaluate(`(function(){
		var result = [];
		var items = document.querySelectorAll('[data-tid="ListItem"]');
		items.forEach(function(item){
			var number = (item.querySelector('[data-tid="WaybillNumber"]') || {}).textContent || '';
			var consignor = (item.querySelector('[data-tid="ConsignorName"]') || {}).textContent || '';
			var consignee = (item.querySelector('[data-tid="ConsigneeName"]') || {}).textContent || '';
			var address = (item.querySelector('[data-tid="Address"]') || {}).textContent || '';
			var cargo = (item.querySelector('[data-tid="CargoValuesCount"]') || {}).textContent || '';
			var desc = (item.querySelector('[data-tid="CargoDescription"]') || {}).textContent || '';
			var cat = '';
			var container = item.closest('[data-tid="TransportationListCategoryHeader"]');
			if(!container){
				var prev = item.parentElement.previousElementSibling;
				if(prev && prev.getAttribute('data-tid') === 'ListHeader') cat = prev.textContent || '';
			}
			// альтернативный поиск категории
			var header = item.parentElement.parentElement.querySelector('[data-tid="ListHeader"]');
			if(header) cat = header.textContent || '';
			
			result.push({
				number: number.trim(),
				consignor: consignor.trim(),
				consignee: consignee.trim(),
				address: address.trim(),
				cargo: cargo.replace(/\s+/g, ' ').trim(),
				description: desc.trim(),
				category: cat.trim()
			});
		});
		return result;
	})()`, &data))
	return data, err
}

func CheckWebsite(ctx context.Context) ([]DeliveryNote, error) {
	err := chromedp.Run(ctx,
		chromedp.Navigate("https://logist.kontur.ru/box-selection"),
		chromedp.WaitReady(`body`),
		chromedp.Sleep(3*time.Second),
	)
	if err != nil {
		return nil, err
	}

	if elementExists(ctx, "Сертификат") {
		fmt.Println("Not logged in, doing login...")
		if err := doLogin(ctx); err != nil {
			return nil, err
		}
	} else {
		fmt.Println("Already logged in")
	}

	fmt.Println("--- Click Перевозчик ---")
	clickElement(ctx, "Перевозчик")
	chromedp.Run(ctx, chromedp.Sleep(3*time.Second))

	notes, err := parseDeliveryNotes(ctx)
	if err != nil {
		return nil, err
	}

	for i, n := range notes {
		fmt.Printf("[%d] %s | %s -> %s | %s | %s\n", i, n.Number, n.Consignor, n.Consignee, n.Address, n.Cargo)
	}

	return notes, nil
}

func RunWorker() {
	ctx, cancel := NewBrowser()
	defer cancel()

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		notes, err := CheckWebsite(ctx)
		if err != nil {
			log.Println(err)
		} else {
			b, _ := json.MarshalIndent(notes, "", "  ")
			log.Printf("Получено %d накладных:\n%s\n", len(notes), string(b))
		}

		<-ticker.C
	}
}

func main() {

	RunWorker()

}
