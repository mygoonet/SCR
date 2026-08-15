package SCRP

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

// clickAtCenter ищет элемент JS-выражением (должно вернуть элемент или null)
// и кликает по его центру реальной мышью — React-меню Контура не реагирует
// на синтетический el.click().
//
// Опрашивает до 10 секунд: элемент может появиться в DOM раньше, чем
// отрисуется (getBoundingClientRect()=0). Раньше клик падал с
// "element not found or invisible" на первой попытке.
func clickAtCenter(ctx context.Context, findJS string) error {
	const pollTimeout = 10 * time.Second
	deadline := time.Now().Add(pollTimeout)
	lastDebug := "no-eval"
	for {
		var posJSON string
		if err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`(function(){
			var el = %s;
			if(!el) return JSON.stringify({ok:false, dbg:'null'});
			var r = el.getBoundingClientRect();
			var dbg = 'el='+el.tagName+' '+el.getAttribute('data-tid')+' w='+r.width+' h='+r.height+' vis='+getComputedStyle(el).visibility+' disp='+getComputedStyle(el).display+' hidden='+(el.hidden||'')+' off='+el.offsetParent;
			if(r.width === 0 || r.height === 0) return JSON.stringify({ok:false, dbg:dbg});
			return JSON.stringify({ok:true, cx:r.left+r.width/2, cy:r.top+r.height/2, dbg:dbg});
		})()`, findJS), &posJSON)); err != nil {
			if ctx.Err() != nil {
				return err
			}
		} else {
			var pos struct {
				Ok  bool    `json:"ok"`
				CX  float64 `json:"cx"`
				CY  float64 `json:"cy"`
				Dbg string  `json:"dbg"`
			}
			json.Unmarshal([]byte(posJSON), &pos)
			lastDebug = pos.Dbg
			if pos.Ok {
				return chromedp.Run(ctx, chromedp.MouseClickXY(pos.CX, pos.CY))
			}
		}

		if time.Now().After(deadline) {
			break
		}
		chromedp.Run(ctx, chromedp.Sleep(300*time.Millisecond))
	}
	log.Printf("CLICK-DEBUG: failed to find clickable: %s | %s", findJS, lastDebug)
	return fmt.Errorf("element not found or invisible (polled %s): %s [last=%s]", pollTimeout, findJS, lastDebug)
}

// waitForJS ждёт, пока JS-условие станет true.
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

// openRowMenu открывает меню "три точки" в строке накладной с номером number.
func openRowMenu(ctx context.Context, number string) error {
	var res string
	if err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`(function(num) {
    var nums = document.querySelectorAll('[data-tid="WaybillNumber"]');

    for (var i = 0; i < nums.length; i++) {
        if (nums[i].textContent.trim() === num) {
            // 1. Поднимаемся до TR
            var tr = nums[i].closest('[data-tid="TableRow"]');
            if (!tr) return 'no table row';

            // 2. Ищем RowActions ВНУТРИ этого TR
            var rowActions = tr.querySelector('[data-tid="RowActions"]');
            if (!rowActions) return 'no row actions';

            // 3. Кликаем по кнопке внутри RowActions
            var btn = rowActions.querySelector('button[aria-controls]');
            if (!btn) return 'no button';

            btn.click();
            return 'ok';
        }
    }

    return 'not found';
})(%q)`, number), &res)); err != nil {
		return err
	}
	if res != "ok" {
		return fmt.Errorf("waybill %q: %s", number, res)
	}
	return nil
}

var screenshotDir = "screenshots"

func init() {
	os.MkdirAll(screenshotDir, 0755)
}

func takeScreenshot(ctx context.Context, name string) {
	var buf []byte
	if err := chromedp.Run(ctx, chromedp.FullScreenshot(&buf, 90)); err != nil {
		log.Printf("screenshot %s: %v", name, err)
		return
	}

	// Если сейчас подписывается накладная — скриншот идёт в её папку
	// (шаг подписания: 1_before, done, ...).
	if l := getActiveLogger(); l != nil {
		l.saveScreenshot(name, buf)
		return
	}

	// Фоновый шаг сессии (логин/nav/таблица): в общую папку + в буфер,
	// чтобы потом скопировать в папку каждой накладной.
	captureFlowShot(name, buf)
	path := filepath.Join(screenshotDir, name+".png")
	if err := os.WriteFile(path, buf, 0644); err != nil {
		log.Printf("save screenshot %s: %v", path, err)
	}
}

// SignDeliveryNote подписывает накладную с номером number из списка
// "Документы на подпись" сертификатом certUser.
func SignDeliveryNote(ctx context.Context, number, certUser string) error {
	takeScreenshot(ctx, number+"_1_before")
	if err := waitForTableRows(ctx); err != nil {
		return err
	}

	// 1. Меню "три точки" в строке накладной
	if err := openRowMenu(ctx, number); err != nil {
		return fmt.Errorf("open row menu: %w", err)
	}

	// 2. Пункт "Подписать без подписи водителя"
	// ВНИМАНИЕ: между waitForJS и clickAtCenter НЕ делать takeScreenshot —
	// FullScreenshot перехватывает фокус и React закрывает попап,
	// элемент исчезает из DOM (querySelector → null).
	signItemJS := `document.querySelector('[data-tid="Popup__root"] [data-tid="SignWithoutDriverSignature"]') !== null`
	if err := waitForJS(ctx, signItemJS, 10*time.Second); err != nil {
		var popupDump string
		chromedp.Run(ctx, chromedp.Evaluate(`(function(){
			var p = document.querySelector('[data-tid="Popup__root"]');
			if(!p) return 'NO Popup__root';
			return 'POPUP: '+p.innerText.replace(/\s+/g,' ').trim().slice(0,200);
		})()`, &popupDump))
		log.Printf("SIGN-DEBUG %s: signItem wait failed. %s", number, popupDump)
		return fmt.Errorf("sign menu item: %w", err)
	}
	// Диагностика: дамп всех пунктов popup-меню перед кликом
	var menuItems string
	chromedp.Run(ctx, chromedp.Evaluate(`(function(){
		var p = document.querySelector('[data-tid="Popup__root"]');
		if(!p) return 'NO Popup__root';
		var items = p.querySelectorAll('[data-tid]');
		var out = [];
		for(var i=0;i<items.length;i++){
			var t = items[i].getAttribute('data-tid');
			if(t && t.indexOf('Popup') === -1)
				out.push(t+'="'+items[i].innerText.replace(/\s+/g,' ').trim().slice(0,60)+'"');
		}
		return out.join(' | ');
	})()`, &menuItems))
	log.Printf("SIGN-DEBUG %s: popup menu items: %s", number, menuItems)

	if err := reactClickEl(ctx, "document.querySelector('[data-tid=\"Popup__root\"] [data-tid=\"SignWithoutDriverSignature\"]')"); err != nil {
		return fmt.Errorf("click sign item: %w", err)
	}
	// Диагностика: что появилось через 2 секунды после клика
	chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
	var afterClick string
	chromedp.Run(ctx, chromedp.Evaluate(`(function(){
		var parts = [];
		parts.push('popup=' + (document.querySelector('[data-tid="Popup__root"]') ? 'YES' : 'NO'));
		parts.push('sidePage=' + (document.querySelector('[data-tid="SidePage__root"]') ? 'YES' : 'NO'));
		parts.push('sidePageFooter=' + (document.querySelector('[data-tid="SidePageFooter__root"]') ? 'YES' : 'NO'));
		var d = document.querySelector('[data-tid="SidePage__root"]');
		if(d) parts.push('sidePageText='+d.innerText.replace(/\s+/g,' ').trim().slice(0,300));
		parts.push('url='+location.href);
		parts.push('bodyHead='+document.body.innerText.replace(/\s+/g,' ').trim().slice(0,300));
		return parts.join('\n');
	})()`, &afterClick))
	log.Printf("SIGN-DEBUG %s: 2s after click: %s", number, afterClick)

	// 3. SidePage "Подписание накладной" -> кнопка "Подписать" в футере
	sidePageJS := `document.querySelector('[data-tid="SidePageFooter__root"] [data-tid="Sign"]') !== null`
	if err := waitForJS(ctx, sidePageJS, 15*time.Second); err != nil {
		var dump string
		chromedp.Run(ctx, chromedp.Evaluate(`(function(){
			var parts = [];
			var sp = document.querySelector('[data-tid="SidePage__root"]');
			parts.push('SidePage__root=' + (sp ? 'YES' : 'NO'));
			var spf = document.querySelector('[data-tid="SidePageFooter__root"]');
			parts.push('SidePageFooter__root=' + (spf ? 'YES' : 'NO'));
			var allSide = document.querySelectorAll('[data-tid*="SidePage"]');
			parts.push('SidePage* count=' + allSide.length);
			for(var i=0;i<allSide.length && i<5;i++) parts.push('  '+allSide[i].getAttribute('data-tid'));
			var popups = document.querySelectorAll('[data-tid="Popup__root"]');
			parts.push('Popup__root count=' + popups.length);
			var modals = document.querySelectorAll('[data-tid*="Modal"], [role="dialog"]');
			parts.push('Modal/dialog count=' + modals.length);
		parts.push('bodyText='+document.body.innerText.replace(/\s+/g,' ').trim().slice(0,500));
		})()`, &dump))
		log.Printf("SIGN-DEBUG %s: sidePage timeout. %s", number, dump)
		takeScreenshot(ctx, number+"_sidepage_fail")
		return fmt.Errorf("sign side page: %w", err)
	}
	signBtnJS := `(function(){
		var f = document.querySelector('[data-tid="SidePageFooter__root"]');
		if(!f) return null;
		var s = f.querySelector('[data-tid="Sign"]');
		return s ? (s.tagName === 'BUTTON' ? s : (s.querySelector('button') || s)) : null;
	})()`
	if err := clickAtCenter(ctx, signBtnJS); err != nil {
		return fmt.Errorf("click Sign: %w", err)
	}

	// 4. Модалка "Выбор сертификата" -> клик по сертификату пользователя -> "Выбрать"
	// Скриншоты между шагами убраны — FullScreenshot перехватывает фокус и
	// закрывает React-попапы/модалки, элементы исчезают из DOM.
	if err := waitForJS(ctx, `document.querySelector('[data-tid^="certificate_"]') !== null`, 30*time.Second); err != nil {
		return fmt.Errorf("certificate modal: %w", err)
	}
	// ВАЖНО: простой .click() (clickAtCenter) НЕ выбирает сертификат —
	// React-выбор привязан к pointerdown/mousedown, а не к click.
	// После одного .click() "Выбрать" остаётся disabled, и весь цикл
	// дальше падает ("не подписывает"). Нужна полная последовательность событий.
	certClickJS := "function reactClick(el){var r=el.getBoundingClientRect();var x=r.x+r.width/2,y=r.y+r.height/2;var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};el.dispatchEvent(new PointerEvent('pointerover',opts));el.dispatchEvent(new MouseEvent('mouseover',opts));el.dispatchEvent(new PointerEvent('pointerdown',opts));el.dispatchEvent(new MouseEvent('mousedown',opts));el.dispatchEvent(new PointerEvent('pointerup',opts));el.dispatchEvent(new MouseEvent('mouseup',opts));el.dispatchEvent(new MouseEvent('click',opts));}" + fmt.Sprintf("(function(user){var certs=document.querySelectorAll('[data-tid^=\"certificate_\"]');for(var i=0;i<certs.length;i++){if(certs[i].textContent.indexOf(user)!==-1){var inner=certs[i].querySelector('div')||certs[i];reactClick(inner);return 'clicked';}}return null;})(%q)", certUser)
	var certRes string
	if err := chromedp.Run(ctx, chromedp.Evaluate(certClickJS, &certRes)); err != nil {
		return fmt.Errorf("click certificate: %w", err)
	}
	if certRes != "clicked" {
		return fmt.Errorf("certificate %q not found", certUser)
	}
	chromedp.Run(ctx, chromedp.Sleep(500*time.Millisecond))

	// Ждём, пока "Выбрать" станет enabled — клик по disabled молча ничего не делает.
	if err := waitForJS(ctx, `(function(){
		var c = document.querySelector('[data-tid="ModalFooter__root"] [data-tid="Choose"], [data-tid="Choose"]');
		if(!c) return false;
		var b = c.tagName === 'BUTTON' ? c : (c.querySelector('button') || c);
		return !b.disabled;
	})()`, 10*time.Second); err != nil {
		return fmt.Errorf("choose button disabled after certificate click: %w", err)
	}
	chooseClickJS := "function reactClick(el){var r=el.getBoundingClientRect();var x=r.x+r.width/2,y=r.y+r.height/2;var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};el.dispatchEvent(new PointerEvent('pointerover',opts));el.dispatchEvent(new MouseEvent('mouseover',opts));el.dispatchEvent(new PointerEvent('pointerdown',opts));el.dispatchEvent(new MouseEvent('mousedown',opts));el.dispatchEvent(new PointerEvent('pointerup',opts));el.dispatchEvent(new MouseEvent('mouseup',opts));el.dispatchEvent(new MouseEvent('click',opts));}" + "(function(){var c=document.querySelector('[data-tid=\"ModalFooter__root\"] [data-tid=\"Choose\"], [data-tid=\"Choose\"]');if(!c) return null;var b=c.tagName==='BUTTON'?c:(c.querySelector('button')||c);reactClick(b);return 'clicked';})()"
	var chooseRes string
	if err := chromedp.Run(ctx, chromedp.Evaluate(chooseClickJS, &chooseRes)); err != nil {
		return fmt.Errorf("click Choose: %w", err)
	}
	if chooseRes != "clicked" {
		return fmt.Errorf("Choose button not found")
	}

	// 5. Модалка "Выбор доверенности" -> доверенность -> "Продолжить"
	if err := waitForJS(ctx, `document.querySelector('[data-tid="representative-poa-list-item"]') !== null`, 30*time.Second); err != nil {
		return fmt.Errorf("poa modal: %w", err)
	}
	poaClickJS := "function reactClick(el){var r=el.getBoundingClientRect();var x=r.x+r.width/2,y=r.y+r.height/2;var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};el.dispatchEvent(new PointerEvent('pointerover',opts));el.dispatchEvent(new MouseEvent('mouseover',opts));el.dispatchEvent(new PointerEvent('pointerdown',opts));el.dispatchEvent(new MouseEvent('mousedown',opts));el.dispatchEvent(new PointerEvent('pointerup',opts));el.dispatchEvent(new MouseEvent('mouseup',opts));el.dispatchEvent(new MouseEvent('click',opts));}" + "(function(){var item=document.querySelector('[data-tid=\"representative-poa-list-item\"]');if(!item) return null;reactClick(item);return 'clicked';})()"
	var poaRes string
	if err := chromedp.Run(ctx, chromedp.Evaluate(poaClickJS, &poaRes)); err != nil {
		return fmt.Errorf("click poa: %w", err)
	}
	if poaRes != "clicked" {
		return fmt.Errorf("poa item not found")
	}
	chromedp.Run(ctx, chromedp.Sleep(500*time.Millisecond))

	// Ждём, пока "Продолжить" станет enabled (доверенность выбрана), затем кликаем.
	if err := waitForJS(ctx, `(function(){
		var b = document.querySelector('[data-tid="continue-with-poa-button"]');
		if(!b) return false;
		var btn = b.tagName === 'BUTTON' ? b : (b.querySelector('button') || b);
		return !btn.disabled;
	})()`, 10*time.Second); err != nil {
		return fmt.Errorf("continue button disabled after poa click: %w", err)
	}
	continueClickJS := "function reactClick(el){var r=el.getBoundingClientRect();var x=r.x+r.width/2,y=r.y+r.height/2;var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};el.dispatchEvent(new PointerEvent('pointerover',opts));el.dispatchEvent(new MouseEvent('mouseover',opts));el.dispatchEvent(new PointerEvent('pointerdown',opts));el.dispatchEvent(new MouseEvent('mousedown',opts));el.dispatchEvent(new PointerEvent('pointerup',opts));el.dispatchEvent(new MouseEvent('mouseup',opts));el.dispatchEvent(new MouseEvent('click',opts));}" + "(function(){var b=document.querySelector('[data-tid=\"continue-with-poa-button\"]');if(!b) return null;var btn=b.tagName==='BUTTON'?b:(b.querySelector('button')||b);reactClick(btn);return 'clicked';})()"
	var contRes string
	if err := chromedp.Run(ctx, chromedp.Evaluate(continueClickJS, &contRes)); err != nil {
		return fmt.Errorf("click continue: %w", err)
	}
	if contRes != "clicked" {
		return fmt.Errorf("continue button not found")
	}

	// 6. Ждём закрытия модалки — подписание выполняется криптоплагином
	if err := waitForJS(ctx,
		`document.querySelector('[data-tid="Modal"][data-tid="root"], [role="dialog"]') === null`,
		90*time.Second); err != nil {
		return fmt.Errorf("signing did not finish: %w", err)
	}

	takeScreenshot(ctx, number+"_done")
	chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
	return nil
}
