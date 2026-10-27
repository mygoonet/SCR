package main

// logindump: диагностика ЗАСТРЕВШЕГО ЛОГИНА. Профиль локально «выпал»,
// поэтому scrpcheck заходит в Login() и застревает на auth.kontur.ru —
// клик по карточке сертификата не завершает редирект обратно на logist.kontur.ru.
//
// Инструмент доходит до страницы выбора сертификата, выгружает ВСЕ элементы,
// чей текст содержит имя пользователя (отсортировано по площади, чтобы найти
// самый глубокий), структуру самой глубокой карточки, все кнопки — затем
// кликает по имени (reactClick dispatchEvent) и показывает, что изменилось:
// URL, новые модалки/кнопки/согласия. Ничего не подписывает.
//
// Запуск:
//   CHROME_PATH=/usr/bin/chromium-gost-stable \
//   USER_DATA_DIR=/home/visa/.config/chromium-gost-scrp \
//   CERT_USER="Сичкарук Евгений Александрович" \
//   ./logindump

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// evalTimeout — дедлайн одного Evaluate: неответившая страница (плагины
// КриптоПро/Госуслуг) должна давать ошибку, а не вечное зависание инструмента.
const evalTimeout = 15 * time.Second

func waitForJS(ctx context.Context, js string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var ok bool
		c, cancel := context.WithTimeout(ctx, evalTimeout)
		err := chromedp.Run(c, chromedp.Evaluate(js, &ok))
		cancel()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		chromedp.Run(ctx, chromedp.Sleep(500*time.Millisecond))
	}
	return fmt.Errorf("timeout %s", timeout)
}

func eval(ctx context.Context, js string) string {
	var out string
	c, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()
	if err := chromedp.Run(c, chromedp.Evaluate(js, &out)); err != nil {
		return "EVAL_ERR: " + err.Error()
	}
	return out
}

// shot — скрин ВИДИМОЙ области с дедлайном. FullScreenshot (captureBeyondViewport)
// на странице входа Контура рисует десятки секунд и вешает инструмент.
func shot(ctx context.Context, path string) {
	var buf []byte
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err := chromedp.Run(c, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		buf, err = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatPng).Do(ctx)
		return err
	}))
	if err == nil {
		if err := os.WriteFile(path, buf, 0644); err == nil {
			log.Printf("screenshot: %s", path)
		}
	} else {
		log.Printf("screenshot %s: %v", path, err)
	}
}

func main() {
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

	// Кэш отключаем один раз.
	chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCacheDisabled(true).Do(ctx)
	}))

	chromedp.Run(ctx, chromedp.Navigate("https://logist.kontur.ru/box-selection"))
	chromedp.Run(ctx, chromedp.Sleep(5*time.Second))

	var url string
	chromedp.Run(ctx, chromedp.Location(&url))
	log.Printf("initial url: %s", url)

	// Есть ли кнопка «Сертификат» (т.е. нужен логин)?
	hasCert := eval(ctx, `(function(){
		var all = document.querySelectorAll('*');
		for(var i=0;i<all.length;i++){
			if(all[i].textContent.replace(/\u00a0/g,' ').trim()==='Сертификат'){ return true; }
		}
		return false;
	})()`)
	log.Printf("has Сертификат button: %s", hasCert)

	// Карточка сертификата может быть видна сразу (таб «Сертификат» активен
	// по умолчанию) — тогда таб не кликаем, как и в SCRP.Login.
	certCard := eval(ctx, `document.querySelector('[data-tid^="certificate_"]') ? 'yes' : 'no'`)
	log.Printf("cert card present: %s", certCard)

	if hasCert == "true" && certCard != "yes" {
		// Клик «Сертификат» (ReactClick exact, min-area).
		clickRes := eval(ctx, `(function(t){
			var all = document.querySelectorAll('*');
			var best = null, bestArea = Infinity;
			for(var i=0;i<all.length;i++){
				var elText = all[i].textContent.replace(/\u00a0/g,' ');
				if(elText.trim() === t){
					var r = all[i].getBoundingClientRect();
					var area = r.width * r.height;
					if(area > 0 && area < bestArea){ bestArea = area; best = all[i]; }
				}
			}
			if(!best) return 'not found';
			var r=best.getBoundingClientRect();var x=r.x+r.width/2,y=r.y+r.height/2;
			var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};
			best.dispatchEvent(new PointerEvent('pointerover',opts));best.dispatchEvent(new MouseEvent('mouseover',opts));best.dispatchEvent(new PointerEvent('pointerdown',opts));best.dispatchEvent(new MouseEvent('mousedown',opts));best.dispatchEvent(new PointerEvent('pointerup',opts));best.dispatchEvent(new MouseEvent('mouseup',opts));best.dispatchEvent(new MouseEvent('click',opts));
			return 'clicked ' + best.tagName;
		})("Сертификат")`)
		log.Printf("click Сертификат: %s", clickRes)

		// Ждём появления имени (до 30с).
		nameFound := eval(ctx, fmt.Sprintf(`(function(t){
			var all = document.querySelectorAll('*');
			for(var i=0;i<all.length;i++){
				if(all[i].textContent.replace(/\u00a0/g,' ').includes(t)) return true;
			}
			return false;
		})(%q)`, certUser))
		log.Printf("name present (immediate): %s", nameFound)
		_ = waitForJS(ctx, fmt.Sprintf(`(function(t){
			var all = document.querySelectorAll('*');
			for(var i=0;i<all.length;i++){
				if(all[i].textContent.replace(/\u00a0/g,' ').includes(t)) return true;
			}
			return false;
		})(%q)`, certUser), 30*time.Second)
		chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
	}

	// ============ ДАМП ФАЗА 1: страница выбора сертификата (до клика по имени) ============
	dump1 := eval(ctx, fmt.Sprintf(`(function(t){
		var out = [];
		out.push('URL: ' + location.href);
		// Все элементы, чей текст СОДЕРЖИТ имя — отсортировано по площади (глубина).
		var all = document.querySelectorAll('*');
		var matches = [];
		for(var i=0;i<all.length;i++){
			if(all[i].textContent.replace(/\u00a0/g,' ').includes(t)){
				var r = all[i].getBoundingClientRect();
				matches.push({
					tag: all[i].tagName,
					area: Math.round(r.width*r.height),
					w: Math.round(r.width), h: Math.round(r.height),
					x: Math.round(r.x), y: Math.round(r.y),
					role: all[i].getAttribute('role'),
					tid: all[i].getAttribute('data-tid'),
					class: (all[i].className && all[i].className.toString ? all[i].className.toString() : '').slice(0,60),
					tabindex: all[i].getAttribute('tabindex'),
					ownText: (all[i].childNodes.length===1 && all[i].childNodes[0].nodeType===3) ? all[i].textContent.trim().slice(0,60) : ''
				});
			}
		}
		matches.sort(function(a,b){ return a.area - b.area; });
		out.push('--- elements containing name (by area asc, top 20) ---');
		for(var m=0;m<matches.length && m<20;m++){
			var e = matches[m];
			out.push(e.tag + ' area=' + e.area + ' [' + e.w + 'x' + e.h + ' @' + e.x + ',' + e.y + '] role=' + (e.role||'-') + ' tid=' + (e.tid||'-') + ' class=' + (e.class||'-') + ' tabindex=' + (e.tabindex||'-') + (e.ownText ? ' OWNTEXT="' + e.ownText + '"' : ''));
		}
		out.push('total matches: ' + matches.length);
		// Самая глубокая карточка — outerHTML + цепочка родителей.
		if(matches.length > 0){
			var deepest = null;
			for(var j=0;j<all.length;j++){
				if(all[j].textContent.replace(/\u00a0/g,' ').includes(t)){
					var rr = all[j].getBoundingClientRect();
					var aa = rr.width*rr.height;
					if(aa>0 && (!deepest || aa < deepest.area)){ deepest = {el:all[j], area:aa}; }
				}
			}
			if(deepest){
				out.push('--- DEEPEST element ---');
				out.push('outerHTML: ' + deepest.el.outerHTML.slice(0,600));
				var chain = []; var cur = deepest.el;
				for(var k=0;k<5 && cur;k++){
					chain.push(cur.tagName + (cur.getAttribute('role')?'[role='+cur.getAttribute('role')+']':'') + (cur.getAttribute('data-tid')?'[tid='+cur.getAttribute('data-tid')+']':''));
					cur = cur.parentElement;
				}
				out.push('parent chain: ' + chain.join(' < '));
			}
		}
		// Все кнопки.
		out.push('--- buttons ---');
		var btns = document.querySelectorAll('button, [role="button"]');
		for(var b=0;b<btns.length && b<40;b++){
			var bt = (btns[b].innerText||'').replace(/\s+/g,' ').trim();
			if(bt) out.push('[' + (btns[b].getAttribute('data-tid')||'-') + '] ' + bt.slice(0,80) + (btns[b].disabled ? ' (disabled)' : ''));
		}
		// Модалки/диалоги.
		out.push('--- dialogs/modals ---');
		var dlgs = document.querySelectorAll('[role="dialog"], [data-tid*="Modal"], [class*="modal"], [class*="Modal"]');
		for(var d=0;d<dlgs.length && d<8;d++){
			out.push('TAG=' + dlgs[d].tagName + ' tid=' + (dlgs[d].getAttribute('data-tid')||'-') + ' TEXT: ' + (dlgs[d].innerText||'').replace(/\s+/g,' ').trim().slice(0,300));
		}
		if(dlgs.length===0) out.push('(none)');
		return out.join('\n');
	})(%q)`, certUser))
	fmt.Println("\n========== FASE 1: cert selection page (before name click) ==========")
	fmt.Println(dump1)
	shot(ctx, "/home/visa/SCRP/screenshots/logindump_phase1.png")

	// ============ КЛИК по самому глубокому элементу с именем ============
	clickName := eval(ctx, fmt.Sprintf(`(function(t){
		var all = document.querySelectorAll('*');
		var best = null, bestArea = Infinity;
		for(var i=0;i<all.length;i++){
			if(all[i].textContent.replace(/\u00a0/g,' ').includes(t)){
				var r = all[i].getBoundingClientRect();
				var area = r.width * r.height;
				if(area > 0 && area < bestArea){ bestArea = area; best = all[i]; }
			}
		}
		if(!best) return 'not found';
		var r=best.getBoundingClientRect();var x=r.x+r.width/2,y=r.y+r.height/2;
		var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};
		best.dispatchEvent(new PointerEvent('pointerover',opts));best.dispatchEvent(new MouseEvent('mouseover',opts));best.dispatchEvent(new PointerEvent('pointerdown',opts));best.dispatchEvent(new MouseEvent('mousedown',opts));best.dispatchEvent(new PointerEvent('pointerup',opts));best.dispatchEvent(new MouseEvent('mouseup',opts));best.dispatchEvent(new MouseEvent('click',opts));
		return 'clicked ' + best.tagName + ' area=' + Math.round(bestArea);
	})(%q)`, certUser))
	log.Printf("click name (deepest): %s", clickName)

	// Ждём до 15с изменения URL или появления нового состояния.
	chromedp.Run(ctx, chromedp.Sleep(8*time.Second))

	// ============ ДАМП ФАЗА 2: после клика по имени ============
	urlBefore := url
	var urlAfter string
	chromedp.Run(ctx, chromedp.Location(&urlAfter))
	dump2 := eval(ctx, fmt.Sprintf(`(function(t){
		var out = [];
		out.push('URL: ' + location.href);
		out.push('--- buttons ---');
		var btns = document.querySelectorAll('button, [role="button"]');
		for(var b=0;b<btns.length && b<40;b++){
			var bt = (btns[b].innerText||'').replace(/\s+/g,' ').trim();
			if(bt) out.push('[' + (btns[b].getAttribute('data-tid')||'-') + '] ' + bt.slice(0,80) + (btns[b].disabled ? ' (disabled)' : ''));
		}
		out.push('--- dialogs/modals ---');
		var dlgs = document.querySelectorAll('[role="dialog"], [data-tid*="Modal"], [class*="modal"], [class*="Modal"]');
		for(var d=0;d<dlgs.length && d<8;d++){
			out.push('TAG=' + dlgs[d].tagName + ' tid=' + (dlgs[d].getAttribute('data-tid')||'-') + ' TEXT: ' + (dlgs[d].innerText||'').replace(/\s+/g,' ').trim().slice(0,300));
		}
		if(dlgs.length===0) out.push('(none)');
		out.push('--- page text (first 800) ---');
		out.push(document.body.innerText.replace(/\s+/g,' ').trim().slice(0,800));
		return out.join('\n');
	})(%q)`, certUser))
	fmt.Println("\n========== FASE 2: after name click ==========")
	fmt.Printf("URL changed: %q -> %q\n", urlBefore, urlAfter)
	fmt.Println(dump2)
	shot(ctx, "/home/visa/SCRP/screenshots/logindump_phase2.png")

	// ESC — закрыть всё, ничего не подписывать.
	chromedp.Run(ctx, chromedp.KeyEvent("Escape"))
	chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
}
