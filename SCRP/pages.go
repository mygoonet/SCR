package SCRP

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

func ClickElement(ctx context.Context, text string) error {
	var result string
	err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(
		`(function(t){
		var all = document.querySelectorAll('*');
		for(var i=0;i<all.length;i++){
			var elText = all[i].textContent.replace(/\u00a0/g, ' ');
			if(elText.trim() === t){
				var el = all[i];
				while(el && el.tagName !== 'A' && el.tagName !== 'BUTTON' && !el.onclick){
					el = el.parentElement;
				}
				if(!el) return 'not clickable';
				el.click();
				return 'clicked ' + el.tagName;
			}
		}
		return 'not found';
	})(%q)`, text), &result))
	if err != nil {
		return err
	}
	if result == "not found" || result == "not clickable" {
		return fmt.Errorf("element %q not found", text)
	}
	return nil
}

func ClickElementContains(ctx context.Context, text string) error {
	var result string
	err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(
		`(function(t){
		var all = document.querySelectorAll('*');
		for(var i=0;i<all.length;i++){
			if(all[i].textContent.replace(/\u00a0/g, ' ').includes(t)){
				var el = all[i];
				while(el && el.tagName !== 'A' && el.tagName !== 'BUTTON' && !el.onclick){
					el = el.parentElement;
				}
				if(!el) return 'not clickable';
				el.click();
				return 'clicked ' + el.tagName;
			}
		}
		return 'not found';
	})(%q)`, text), &result))
	if err != nil {
		return err
	}
	if result == "not found" || result == "not clickable" {
		return fmt.Errorf("element containing %q not found", text)
	}
	return nil
}

func ElementExists(ctx context.Context, text string) bool {
	var exists bool
	chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(
		`(function(t){
		var all = document.querySelectorAll('*');
		for(var i=0;i<all.length;i++){
			if(all[i].textContent.replace(/\u00a0/g, ' ').trim() === t) return true;
		}
		return false;
	})(%q)`, text), &exists))
	return exists
}

func ElementExistsContains(ctx context.Context, text string) bool {
	var exists bool
	chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(
		`(function(t){
		var all = document.querySelectorAll('*');
		for(var i=0;i<all.length;i++){
			if(all[i].textContent.replace(/\u00a0/g, ' ').includes(t)) return true;
		}
		return false;
	})(%q)`, text), &exists))
	return exists
}

func PageText(ctx context.Context) string {
	var text string
	chromedp.Run(ctx, chromedp.Evaluate(`document.body.innerText`, &text))
	return text
}

func WaitForPage(ctx context.Context) error {
	return chromedp.Run(ctx,
		chromedp.WaitReady(`body`),
		chromedp.Sleep(3*time.Second),
	)
}

// reactWaitForText ждёт (до 30с) появления элемента с нужным текстом.
// exact=true — точное совпадение, false — вхождение.
func reactWaitForText(ctx context.Context, text string, exact bool) error {
	js := fmt.Sprintf(`(function(t,exact){
		var all = document.querySelectorAll('*');
		for(var i=0;i<all.length;i++){
			var elText = all[i].textContent.replace(/\\u00a0/g, ' ');
			if(exact ? elText.trim() === t : elText.includes(t)) return true;
		}
		return false;
	})(%q,%v)`, text, exact)
	return waitForJS(ctx, js, 30*time.Second)
}

// ReactClick — клик по элементу с ТОЧНЫМ текстом через полную
// последовательность событий (pointerover→mouseover→pointerdown→mousedown→
// pointerup→mouseup→click), как в reactClick из sign.go. Простой .click()
// на React-элементах (div/span с addEventListener) не срабатывает.
// ВАЖНО: среди всех элементов с совпадающим текстом выбираем САМЫЙ ГЛУБОКИЙ
// (минимальная площадь getBoundingClientRect) — иначе first-match может быть
// <HTML>/<BODY>, содержащий весь текст страницы, и клик "пройдёт мимо".
func ReactClick(ctx context.Context, text string) error {
	if err := reactWaitForText(ctx, text, true); err != nil {
		return fmt.Errorf("wait %q: %w", text, err)
	}
	js := fmt.Sprintf(`(function(t){
		var all = document.querySelectorAll('*');
		var best = null, bestArea = Infinity;
		for(var i=0;i<all.length;i++){
			var elText = all[i].textContent.replace(/\\u00a0/g, ' ');
			if(elText.trim() === t){
				var r = all[i].getBoundingClientRect();
				var area = r.width * r.height;
				if(area > 0 && area < bestArea){ bestArea = area; best = all[i]; }
			}
		}
		if(!best) return 'not found';
		var r=best.getBoundingClientRect();var x=r.x+r.width/2,y=r.y+r.height/2;var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};
		best.dispatchEvent(new PointerEvent('pointerover',opts));best.dispatchEvent(new MouseEvent('mouseover',opts));best.dispatchEvent(new PointerEvent('pointerdown',opts));best.dispatchEvent(new MouseEvent('mousedown',opts));best.dispatchEvent(new PointerEvent('pointerup',opts));best.dispatchEvent(new MouseEvent('mouseup',opts));best.dispatchEvent(new MouseEvent('click',opts));
		return 'clicked ' + best.tagName;
	})(%q)`, text)
	var result string
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return err
	}
	if result == "not found" {
		return fmt.Errorf("element %q not found", text)
	}
	return nil
}

// ReactClickContains — клик по элементу, ЧЕЙ ТЕКСТ СОДЕРЖИТ нужную строку
// (для имени пользователя в списке сертификатов). Среди совпадений выбираем
// самый глубокий (минимальная площадь) — это сама карточка, а не <HTML>.
func ReactClickContains(ctx context.Context, text string) error {
	if err := reactWaitForText(ctx, text, false); err != nil {
		return fmt.Errorf("wait %q: %w", text, err)
	}
	js := fmt.Sprintf(`(function(t){
		var all = document.querySelectorAll('*');
		var best = null, bestArea = Infinity;
		for(var i=0;i<all.length;i++){
			if(all[i].textContent.replace(/\\u00a0/g, ' ').includes(t)){
				var r = all[i].getBoundingClientRect();
				var area = r.width * r.height;
				if(area > 0 && area < bestArea){ bestArea = area; best = all[i]; }
			}
		}
		if(!best) return 'not found';
		var r=best.getBoundingClientRect();var x=r.x+r.width/2,y=r.y+r.height/2;var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};
		best.dispatchEvent(new PointerEvent('pointerover',opts));best.dispatchEvent(new MouseEvent('mouseover',opts));best.dispatchEvent(new PointerEvent('pointerdown',opts));best.dispatchEvent(new MouseEvent('mousedown',opts));best.dispatchEvent(new PointerEvent('pointerup',opts));best.dispatchEvent(new MouseEvent('mouseup',opts));best.dispatchEvent(new MouseEvent('click',opts));
		return 'clicked ' + best.tagName;
	})(%q)`, text)
	var result string
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return err
	}
	if result == "not found" {
		return fmt.Errorf("element containing %q not found", text)
	}
	return nil
}

// DumpCertCard возвращает структуру карточки сертификата, чей текст
// содержит certUser: тег, role, data-tid, class, наличие обработчиков
// (onclick) и координаты. Нужно для диагностики, почему клик по имени
// на странице входа не завершает вход.
func DumpCertCard(ctx context.Context, certUser string) string {
	var out string
	js := fmt.Sprintf(`(function(t){
		var all = document.querySelectorAll('*');
		for(var i=0;i<all.length;i++){
			if(all[i].textContent.replace(/\\u00a0/g,' ').includes(t)){
				var el = all[i];
				var chain = [];
				var cur = el;
				for(var k=0;k<4 && cur;k++){
					chain.push(cur.tagName + (cur.getAttribute('role')?'[role='+cur.getAttribute('role')+']':'') + (cur.getAttribute('data-tid')?'[data-tid='+cur.getAttribute('data-tid')+']':'') + '[class='+(cur.className&&cur.className.toString?cur.className.toString().slice(0,40):'')+']');
					cur = cur.parentElement;
				}
				var r = el.getBoundingClientRect();
				return JSON.stringify({
					text: el.textContent.replace(/\\s+/g,' ').trim().slice(0,80),
					tag: el.tagName,
					role: el.getAttribute('role'),
					dataTid: el.getAttribute('data-tid'),
					hasOnclick: typeof el.onclick === 'function',
					rect: {x:Math.round(r.x),y:Math.round(r.y),w:Math.round(r.width),h:Math.round(r.height)},
					matchChain: chain
				});
			}
		}
		return 'NOT_FOUND';
	})(%q)`, certUser)
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &out)); err != nil {
		return "DUMP_ERR: " + err.Error()
	}
	return out
}
