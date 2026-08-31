package main

// poadump: воспроизводит путь подписания до клика «Выбрать» в модалке
// сертификата, затем кликает «Выбрать» и выгружает всё, что появилось —
// для диагностики сломанного селектора representative-poa-list-item.
//
// Запуск:
//   CHROME_PATH=/opt/chromium-gost/chromium-gost \
//   USER_DATA_DIR=/home/visa/.config/chromium-gost-scrp \
//   CERT_USER="Сичкарук Евгений Александрович" \
//   ./poadump [номер]

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

func reactClickEl(ctx context.Context, selector string) error {
	js := "(function(){function reactClick(el){var r=el.getBoundingClientRect();" +
		"var x=r.x+r.width/2,y=r.y+r.height/2;" +
		"var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};el.dispatchEvent(new PointerEvent('pointerover',opts));el.dispatchEvent(new MouseEvent('mouseover',opts));el.dispatchEvent(new PointerEvent('pointerdown',opts));el.dispatchEvent(new MouseEvent('mousedown',opts));el.dispatchEvent(new PointerEvent('pointerup',opts));el.dispatchEvent(new MouseEvent('mouseup',opts));el.dispatchEvent(new MouseEvent('click',opts));}var el=" + selector + ";if(!el)return 'NOT_FOUND';reactClick(el);return 'OK';})()"
	var res string
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &res)); err != nil {
		return err
	}
	if res != "OK" {
		return fmt.Errorf("element not found: %s", selector)
	}
	return nil
}

func clickAtCenter(ctx context.Context, findJS string) error {
	const pollTimeout = 10 * time.Second
	deadline := time.Now().Add(pollTimeout)
	for {
		var posJSON string
		if err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`(function(){
			var el = %s;
			if(!el) return JSON.stringify({ok:false});
			var r = el.getBoundingClientRect();
			if(r.width === 0 || r.height === 0) return JSON.stringify({ok:false});
			return JSON.stringify({ok:true, cx:r.left+r.width/2, cy:r.top+r.height/2});
		})()`, findJS), &posJSON)); err != nil {
			if ctx.Err() != nil {
				return err
			}
		} else {
			var pos struct {
				Ok bool    `json:"ok"`
				CX float64 `json:"cx"`
				CY float64 `json:"cy"`
			}
			if err := jsonUnmarshal(posJSON, &pos); err == nil && pos.Ok {
				return chromedp.Run(ctx, chromedp.MouseClickXY(pos.CX, pos.CY))
			}
		}
		if time.Now().After(deadline) {
			break
		}
		chromedp.Run(ctx, chromedp.Sleep(300*time.Millisecond))
	}
	return fmt.Errorf("element not found or invisible (polled %s)", pollTimeout)
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

	// Кэш отключаем один раз на всю сессию (как в initSession).
	chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCacheDisabled(true).Do(ctx)
	}))

	// Логин (профиль уже авторизован) + переход в Перевозчика.
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

	var url string
	chromedp.Run(ctx, chromedp.Location(&url))
	log.Printf("url after nav: %s", url)

	// Ждём таблицу.
	if err := waitForJS(ctx, `document.querySelectorAll('[data-tid="TableRow"]').length > 0`, 30*time.Second); err != nil {
		log.Fatalf("table rows: %v", err)
	}
	log.Printf("table OK, url=%s", url)

	// Ждём конкретный номер накладной в таблице.
	if err := waitForJS(ctx, fmt.Sprintf(`(function(){var n=document.querySelectorAll('[data-tid="WaybillNumber"]');for(var i=0;i<n.length;i++){if(n[i].textContent.trim()===%q)return true;}return false;})()`, number), 30*time.Second); err != nil {
		log.Fatalf("waybill %s not in table: %v", number, err)
	}
	log.Printf("waybill %s found in table", number)

	// 1. Меню «три точки» в строке накладной.
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
	log.Println("=== 1. row menu opened")

	// 2. «Подписать без подписи водителя».
	if err := waitForJS(ctx, `document.querySelector('[data-tid="Popup__root"] [data-tid="SignWithoutDriverSignature"]') !== null`, 10*time.Second); err != nil {
		log.Fatalf("sign menu item: %v", err)
	}
	if err := reactClickEl(ctx, `document.querySelector('[data-tid="Popup__root"] [data-tid="SignWithoutDriverSignature"]')`); err != nil {
		log.Fatalf("click sign item: %v", err)
	}
	log.Println("=== 2. clicked sign item")

	// 3. SidePage -> «Подписать».
	if err := waitForJS(ctx, `document.querySelector('[data-tid="SidePageFooter__root"] [data-tid="Sign"]') !== null`, 15*time.Second); err != nil {
		log.Fatalf("side page: %v", err)
	}
	signBtnJS := `(function(){
		var f = document.querySelector('[data-tid="SidePageFooter__root"]');
		if(!f) return null;
		var s = f.querySelector('[data-tid="Sign"]');
		return s ? (s.tagName === 'BUTTON' ? s : (s.querySelector('button') || s)) : null;
	})()`
	if err := clickAtCenter(ctx, signBtnJS); err != nil {
		log.Fatalf("click Sign: %v", err)
	}
	log.Println("=== 3. clicked Sign button")

	// 4. Модалка сертификата -> сертификат -> «Выбрать».
	if err := waitForJS(ctx, `document.querySelector('[data-tid^="certificate_"]') !== null`, 30*time.Second); err != nil {
		log.Fatalf("certificate modal: %v", err)
	}
	log.Println("=== 4. certificate modal visible")

	certClickJS := "function reactClick(el){var r=el.getBoundingClientRect();var x=r.x+r.width/2,y=r.y+r.height/2;var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};el.dispatchEvent(new PointerEvent('pointerover',opts));el.dispatchEvent(new MouseEvent('mouseover',opts));el.dispatchEvent(new PointerEvent('pointerdown',opts));el.dispatchEvent(new MouseEvent('mousedown',opts));el.dispatchEvent(new PointerEvent('pointerup',opts));el.dispatchEvent(new MouseEvent('mouseup',opts));el.dispatchEvent(new MouseEvent('click',opts));}" + fmt.Sprintf("(function(user){var certs=document.querySelectorAll('[data-tid^=\"certificate_\"]');for(var i=0;i<certs.length;i++){if(certs[i].textContent.indexOf(user)!==-1){var inner=certs[i].querySelector('div')||certs[i];reactClick(inner);return 'clicked';}}return null;})(%q)", certUser)
	var certRes string
	if err := chromedp.Run(ctx, chromedp.Evaluate(certClickJS, &certRes)); err != nil {
		log.Fatalf("click certificate: %v", err)
	}
	fmt.Println("=== cert click:", certRes)

	if err := waitForJS(ctx, `(function(){
		var c = document.querySelector('[data-tid="ModalFooter__root"] [data-tid="Choose"], [data-tid="Choose"]');
		if(!c) return false;
		var b = c.tagName === 'BUTTON' ? c : (c.querySelector('button') || c);
		return !b.disabled;
	})()`, 10*time.Second); err != nil {
		log.Fatalf("choose disabled: %v", err)
	}
	fmt.Println("=== Choose enabled, clicking...")

	chooseClickJS := "function reactClick(el){var r=el.getBoundingClientRect();var x=r.x+r.width/2,y=r.y+r.height/2;var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};el.dispatchEvent(new PointerEvent('pointerover',opts));el.dispatchEvent(new MouseEvent('mouseover',opts));el.dispatchEvent(new PointerEvent('pointerdown',opts));el.dispatchEvent(new MouseEvent('mousedown',opts));el.dispatchEvent(new PointerEvent('pointerup',opts));el.dispatchEvent(new MouseEvent('mouseup',opts));el.dispatchEvent(new MouseEvent('click',opts));}" + "(function(){var c=document.querySelector('[data-tid=\"ModalFooter__root\"] [data-tid=\"Choose\"], [data-tid=\"Choose\"]');if(!c) return null;var b=c.tagName==='BUTTON'?c:(c.querySelector('button')||c);reactClick(b);return 'clicked';})()"
	var chooseRes string
	if err := chromedp.Run(ctx, chromedp.Evaluate(chooseClickJS, &chooseRes)); err != nil {
		log.Fatalf("click Choose: %v", err)
	}
	fmt.Println("=== choose click:", chooseRes)

	// Ждём до 40с появления ЧЕГО-ЛИБО нового (модалка POA или что угодно).
	time.Sleep(40 * time.Second)

	// ДАМП состояния.
	dumpJS := `(function(){
		var out = [];
		out.push('URL: ' + location.href);
		var tids = {};
		var all = document.querySelectorAll('[data-tid]');
		for(var i=0;i<all.length;i++){
			var t = all[i].getAttribute('data-tid');
			tids[t] = (tids[t]||0)+1;
		}
		out.push('--- unique data-tid (' + all.length + ' elements) ---');
		var keys = Object.keys(tids).sort();
		for(var k=0;k<keys.length;k++) out.push(keys[k] + ' x' + tids[keys[k]]);
		out.push('--- poa-ish elements ---');
		var found = 0;
		for(var j=0;j<all.length;j++){
			var t2 = (all[j].getAttribute('data-tid')||'').toLowerCase();
			if(t2.indexOf('poa')!==-1 || t2.indexOf('representative')!==-1 || t2.indexOf('deed')!==-1){
				out.push(all[j].outerHTML.slice(0,400));
				found++;
			}
		}
		if(found===0) out.push('(none)');
		out.push('--- dialogs/modals ---');
		var dlgs = document.querySelectorAll('[role="dialog"], [data-tid*="Modal"], [class*="modal"], [class*="Modal"]');
		for(var m=0;m<dlgs.length && m<10;m++){
			var d = dlgs[m];
			out.push('TAG=' + d.tagName + ' tid=' + (d.getAttribute('data-tid')||'-') + ' class=' + (d.className||'').toString().slice(0,120));
			out.push('TEXT: ' + (d.innerText||'').replace(/\\s+/g,' ').trim().slice(0,500));
		}
		if(dlgs.length===0) out.push('(none)');
		out.push('--- buttons ---');
		var btns = document.querySelectorAll('button');
		for(var b=0;b<btns.length && b<40;b++){
			var bt = (btns[b].innerText||'').replace(/\\s+/g,' ').trim();
			if(bt) out.push('[' + (btns[b].getAttribute('data-tid')||'-') + '] ' + bt.slice(0,80) + (btns[b].disabled ? ' (disabled)' : ''));
		}
		return out.join('\\n');
	})()`
	var dump string
	if err := chromedp.Run(ctx, chromedp.Evaluate(dumpJS, &dump)); err != nil {
		log.Fatalf("dump: %v", err)
	}
	fmt.Println(dump)

	var buf []byte
	if err := chromedp.Run(ctx, chromedp.FullScreenshot(&buf, 90)); err == nil {
		if err := os.WriteFile("/home/visa/SCRP/screenshots/poadump_after_choose.png", buf, 0644); err == nil {
			fmt.Println("screenshot saved: /home/visa/SCRP/screenshots/poadump_after_choose.png")
		}
	}
}
