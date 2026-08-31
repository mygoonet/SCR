package main

// poaverify: доходит до модалки «Выбор доверенности», кликает новый селектор
// scoped-poa-list-item и проверяет, что continue-with-poa-button становится
// enabled. НЕ нажимает «Сохранить» — подписание не выполняется.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func jsonUnmarshal(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}

func waitForJS(ctx context.Context, js string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var ok bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(js, &ok)); err != nil {
			return err
		}
		if ok {
			return nil
		}
		chromedp.Run(ctx, chromedp.Sleep(500*time.Millisecond))
	}
	return fmt.Errorf("timeout %s waiting for: %s", timeout, js)
}

func main() {
	number := "0000242398"
	if len(os.Args) > 1 {
		number = os.Args[1]
	}
	chromePath := os.Getenv("CHROME_PATH")
	userDataDir := os.Getenv("USER_DATA_DIR")
	certUser := os.Getenv("CERT_USER")
	if chromePath == "" || certUser == "" {
		log.Fatal("CHROME_PATH and CERT_USER env vars are required")
	}
	if userDataDir == "" {
		userDataDir = "/home/visa/.config/chromium-gost-scrp"
	}

	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(chromePath),
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
		chromedp.Flag("window-size", "1920,1080"),
		chromedp.Flag("user-data-dir", userDataDir),
		chromedp.Flag("disk-cache-size", "0"),
		chromedp.Flag("disable-application-cache", true),
	}
	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	chromedp.Run(ctx, chromedp.Navigate("about:blank"))
	chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCacheDisabled(true).Do(ctx)
	}))

	chromedp.Run(ctx, chromedp.Navigate("https://logist.kontur.ru/box-selection"))
	chromedp.Run(ctx, chromedp.Sleep(3*time.Second))
	var carrierClicked string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		chromedp.Run(ctx, chromedp.Evaluate(`(function(){
			var btns = document.querySelectorAll('[data-tid="OrgItemRoleButton"] button');
			for(var i=0;i<btns.length;i++){
				if(btns[i].textContent.trim() === 'Перевозчик'){
					btns[i].click(); return 'clicked';
				}
			}
			return 'not found';
		})()`, &carrierClicked))
		if carrierClicked == "clicked" {
			break
		}
		chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
	}
	log.Printf("carrier click: %s", carrierClicked)

	if err := waitForJS(ctx, `document.querySelectorAll('[data-tid="TableRow"]').length > 0`, 30*time.Second); err != nil {
		log.Fatalf("table rows: %v", err)
	}
	if err := waitForJS(ctx, fmt.Sprintf(`(function(){var n=document.querySelectorAll('[data-tid="WaybillNumber"]');for(var i=0;i<n.length;i++){if(n[i].textContent.trim()===%q)return true;}return false;})()`, number), 30*time.Second); err != nil {
		log.Fatalf("waybill %s not in table: %v", number, err)
	}
	log.Printf("waybill %s found", number)

	var res string
	if err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`(function(num) {
		var nums = document.querySelectorAll('[data-tid="WaybillNumber"]');
		for (var i = 0; i < nums.length; i++) {
			if (nums[i].textContent.trim() === num) {
				var tr = nums[i].closest('[data-tid="TableRow"]');
				if (!tr) return 'no table row';
				var rowActions = tr.querySelector('[data-tid="RowActions"]');
				if (!rowActions) return 'no row actions';
				var btn = rowActions.querySelector('button[aria-controls]');
				if (!btn) return 'no button';
				btn.click();
				return 'ok';
			}
		}
		return 'not found';
	})(%q)`, number), &res)); err != nil {
		log.Fatalf("open row menu: %v", err)
	}
	if res != "ok" {
		log.Fatalf("row menu: %s", res)
	}

	if err := waitForJS(ctx, `document.querySelector('[data-tid="Popup__root"] [data-tid="SignWithoutDriverSignature"]') !== null`, 10*time.Second); err != nil {
		log.Fatalf("sign menu item: %v", err)
	}
	reactClickEl := func(selector string) {
		js := "(function(){function reactClick(el){var r=el.getBoundingClientRect();" +
			"var x=r.x+r.width/2,y=r.y+r.height/2;" +
			"var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};el.dispatchEvent(new PointerEvent('pointerover',opts));el.dispatchEvent(new MouseEvent('mouseover',opts));el.dispatchEvent(new PointerEvent('pointerdown',opts));el.dispatchEvent(new MouseEvent('mousedown',opts));el.dispatchEvent(new PointerEvent('pointerup',opts));el.dispatchEvent(new MouseEvent('mouseup',opts));el.dispatchEvent(new MouseEvent('click',opts));}var el=" + selector + ";if(!el)return 'NOT_FOUND';reactClick(el);return 'OK';})()"
		var r string
		chromedp.Run(ctx, chromedp.Evaluate(js, &r))
		log.Printf("reactClick %s -> %s", selector[:40], r)
	}
	reactClickEl(`document.querySelector('[data-tid="Popup__root"] [data-tid="SignWithoutDriverSignature"]')`)

	if err := waitForJS(ctx, `document.querySelector('[data-tid="SidePageFooter__root"] [data-tid="Sign"]') !== null`, 15*time.Second); err != nil {
		log.Fatalf("side page: %v", err)
	}
	// Клик по «Подписать» — реальной мышью (как в clickAtCenter).
	var posJSON string
	chromedp.Run(ctx, chromedp.Evaluate(`(function(){
		var f = document.querySelector('[data-tid="SidePageFooter__root"]');
		if(!f) return JSON.stringify({ok:false});
		var s = f.querySelector('[data-tid="Sign"]');
		if(!s) return JSON.stringify({ok:false});
		var r = s.getBoundingClientRect();
		return JSON.stringify({ok:true, cx:r.left+r.width/2, cy:r.top+r.height/2});
	})()`, &posJSON))
	var pos struct {
		Ok bool    `json:"ok"`
		CX float64 `json:"cx"`
		CY float64 `json:"cy"`
	}
	if err := jsonUnmarshal(posJSON, &pos); err != nil || !pos.Ok {
		log.Fatalf("sign button pos: %s", posJSON)
	}
	chromedp.Run(ctx, chromedp.MouseClickXY(pos.CX, pos.CY))
	log.Println("clicked Sign")

	if err := waitForJS(ctx, `document.querySelector('[data-tid^="certificate_"]') !== null`, 30*time.Second); err != nil {
		log.Fatalf("certificate modal: %v", err)
	}
	certClickJS := "function reactClick(el){var r=el.getBoundingClientRect();var x=r.x+r.width/2,y=r.y+r.height/2;var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};el.dispatchEvent(new PointerEvent('pointerover',opts));el.dispatchEvent(new MouseEvent('mouseover',opts));el.dispatchEvent(new PointerEvent('pointerdown',opts));el.dispatchEvent(new MouseEvent('mousedown',opts));el.dispatchEvent(new PointerEvent('pointerup',opts));el.dispatchEvent(new MouseEvent('mouseup',opts));el.dispatchEvent(new MouseEvent('click',opts));}" + fmt.Sprintf("(function(user){var certs=document.querySelectorAll('[data-tid^=\"certificate_\"]');for(var i=0;i<certs.length;i++){if(certs[i].textContent.indexOf(user)!==-1){var inner=certs[i].querySelector('div')||certs[i];reactClick(inner);return 'clicked';}}return null;})(%q)", certUser)
	var certRes string
	chromedp.Run(ctx, chromedp.Evaluate(certClickJS, &certRes))
	log.Printf("cert click: %s", certRes)

	if err := waitForJS(ctx, `(function(){
		var c = document.querySelector('[data-tid="ModalFooter__root"] [data-tid="Choose"], [data-tid="Choose"]');
		if(!c) return false;
		var b = c.tagName === 'BUTTON' ? c : (c.querySelector('button') || c);
		return !b.disabled;
	})()`, 10*time.Second); err != nil {
		log.Fatalf("choose disabled: %v", err)
	}
	reactClickEl(`(function(){var c=document.querySelector('[data-tid="ModalFooter__root"] [data-tid="Choose"], [data-tid="Choose"]');if(!c) return null;return c.tagName==="BUTTON"?c:(c.querySelector("button")||c);})()`)
	log.Println("clicked Choose")

	// Ждём модалку доверенности по НОВОМУ селектору.
	if err := waitForJS(ctx, `document.querySelector('[data-tid="scoped-poa-list-item"]') !== null`, 30*time.Second); err != nil {
		log.Fatalf("POA modal (new selector): %v", err)
	}
	log.Println("=== POA modal visible with scoped-poa-list-item")

	// Клик по НОВОМУ селектору пункта доверенности.
	reactClickEl(`document.querySelector('[data-tid="scoped-poa-list-item"]')`)
	log.Println("clicked scoped-poa-list-item")

	// Проверяем, что continue-with-poa-button стал enabled.
	if err := waitForJS(ctx, `(function(){
		var b = document.querySelector('[data-tid="continue-with-poa-button"]');
		if(!b) return false;
		var btn = b.tagName === 'BUTTON' ? b : (b.querySelector('button') || b);
		return !btn.disabled;
	})()`, 10*time.Second); err != nil {
		log.Fatalf("continue button still disabled after new-selector click: %v", err)
	}
	log.Println("=== continue-with-poa-button ENABLED — новый селектор работает")

	// НЕ кликаем «Сохранить» — закрываем всё ESC, ничего не подписывая.
	for i := 0; i < 3; i++ {
		chromedp.Run(ctx, chromedp.KeyEvent("\x1b"))
		chromedp.Run(ctx, chromedp.Sleep(500*time.Millisecond))
	}
	log.Println("closed with ESC, nothing signed")
}
