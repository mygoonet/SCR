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
// на React-элементах (div/span с addEventListener) не срабатывает, а
// подъём к "кликабельному" предку (A/BUTTON/role=button) не находит его,
// т.к. обработчик навешан на div строки. Поэтому кликаем САМ элемент с
// текстом — событие с bubbles:true всплывёт до React-обработчика строки.
func ReactClick(ctx context.Context, text string) error {
	if err := reactWaitForText(ctx, text, true); err != nil {
		return fmt.Errorf("wait %q: %w", text, err)
	}
	js := fmt.Sprintf(`(function(t){
		var all = document.querySelectorAll('*');
		for(var i=0;i<all.length;i++){
			var elText = all[i].textContent.replace(/\\u00a0/g, ' ');
			if(elText.trim() === t){
				var el = all[i];
				var r=el.getBoundingClientRect();var x=r.x+r.width/2,y=r.y+r.height/2;var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};
				el.dispatchEvent(new PointerEvent('pointerover',opts));el.dispatchEvent(new MouseEvent('mouseover',opts));el.dispatchEvent(new PointerEvent('pointerdown',opts));el.dispatchEvent(new MouseEvent('mousedown',opts));el.dispatchEvent(new PointerEvent('pointerup',opts));el.dispatchEvent(new MouseEvent('mouseup',opts));el.dispatchEvent(new MouseEvent('click',opts));
				return 'clicked ' + el.tagName;
			}
		}
		return 'not found';
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
// (для имени пользователя в списке сертификатов). Та же reactClick-логика:
// кликаем сам элемент с текстом, событие всплывает до React-обработчика.
func ReactClickContains(ctx context.Context, text string) error {
	if err := reactWaitForText(ctx, text, false); err != nil {
		return fmt.Errorf("wait %q: %w", text, err)
	}
	js := fmt.Sprintf(`(function(t){
		var all = document.querySelectorAll('*');
		for(var i=0;i<all.length;i++){
			if(all[i].textContent.replace(/\\u00a0/g, ' ').includes(t)){
				var el = all[i];
				var r=el.getBoundingClientRect();var x=r.x+r.width/2,y=r.y+r.height/2;var opts={bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button:0};
				el.dispatchEvent(new PointerEvent('pointerover',opts));el.dispatchEvent(new MouseEvent('mouseover',opts));el.dispatchEvent(new PointerEvent('pointerdown',opts));el.dispatchEvent(new MouseEvent('mousedown',opts));el.dispatchEvent(new PointerEvent('pointerup',opts));el.dispatchEvent(new MouseEvent('mouseup',opts));el.dispatchEvent(new MouseEvent('click',opts));
				return 'clicked ' + el.tagName;
			}
		}
		return 'not found';
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
