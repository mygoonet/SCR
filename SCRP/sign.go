package SCRP

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

func OpenWaybillSidePage(ctx context.Context, number string) error {
	err := chromedp.Run(ctx, chromedp.Tasks{
		chromedp.Sleep(1 * time.Second),
		waitForItems(),
		clickWaybill(number),
		chromedp.Sleep(2 * time.Second),
		waitForSidePage(),
		chromedp.Sleep(1 * time.Second),
	})
	return err
}

func waitForItems() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		for i := 0; i < 30; i++ {
			var n int
			if err := chromedp.Evaluate(
				`document.querySelectorAll('[data-tid="ListItem"]').length`, &n).Do(ctx); err != nil {
				return err
			}
			if n > 0 {
				return nil
			}
			chromedp.Sleep(1 * time.Second).Do(ctx)
		}
		return fmt.Errorf("no ListItems found on page")
	}
}

func clickWaybill(number string) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var result string
		if err := chromedp.Evaluate(fmt.Sprintf(
			`(function(num){
			var items = document.querySelectorAll('[data-tid="ListItem"]');
			for(var i=0;i<items.length;i++){
				var numEl = items[i].querySelector('[data-tid="WaybillNumber"]');
				if(numEl && numEl.textContent.trim() === num){
					items[i].click();
					return JSON.stringify({ok: true});
				}
			}
			return JSON.stringify({err: 'not found'});
		})(%q)`, number), &result).Do(ctx); err != nil {
			return err
		}
		var r struct {
			Ok  bool   `json:"ok"`
			Err string `json:"err"`
		}
		json.Unmarshal([]byte(result), &r)
		if r.Err != "" {
			return fmt.Errorf("waybill %q not found", number)
		}
		return nil
	}
}

func waitForSidePage() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		for i := 0; i < 30; i++ {
			var hasSide bool
			if err := chromedp.Evaluate(
				`document.querySelector('[data-tid="SidePage__root"]') !== null`, &hasSide).Do(ctx); err != nil {
				return err
			}
			if hasSide {
				return nil
			}
			chromedp.Sleep(1 * time.Second).Do(ctx)
		}
		var text string
		chromedp.Evaluate(`document.body.innerText.substring(0, 1000)`, &text).Do(ctx)
		return fmt.Errorf("side page did not appear. text: %q", text)
	}
}

func clickPopupMenu() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var result string
		if err := chromedp.Evaluate(`(function(){
			var btn = document.querySelector('button[aria-controls*="Popup"]');
			if(btn){
				btn.click();
				return JSON.stringify({ok: true, source: 'global'});
			}
			var sp = document.querySelector('[data-tid="SidePage__root"]');
			if(sp){
				btn = sp.querySelector('button[aria-controls*="Popup"]');
				if(btn){
					btn.click();
					return JSON.stringify({ok: true, source: 'sidepage'});
				}
			}
			return JSON.stringify({ok: false});
		})()`, &result).Do(ctx); err != nil {
			return err
		}
		var r struct {
			Ok     bool   `json:"ok"`
			Source string `json:"source"`
		}
		json.Unmarshal([]byte(result), &r)
		if !r.Ok {
			return fmt.Errorf("popup menu button (aria-controls*=Popup) not found — waybill may not be in signable state")
		}
		return nil
	}
}

func waitForPopup() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		for i := 0; i < 20; i++ {
			var found bool
			if err := chromedp.Evaluate(`(function(){
				var items = document.querySelectorAll('[role="menuitem"], [data-tid*="MenuItem"], [data-tid*="Popup"] *');
				if(items.length > 0) return true;
				var popup = document.querySelector('[role="menu"], [role="listbox"], [data-tid*="Popup__"]');
				return popup !== null;
			})()`, &found).Do(ctx); err != nil {
				return err
			}
			if found {
				return nil
			}
			chromedp.Sleep(500 * time.Millisecond).Do(ctx)
		}
		return fmt.Errorf("popup menu did not appear after clicking trigger button")
	}
}

func clickPopupOption(text string) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var result string
		if err := chromedp.Evaluate(fmt.Sprintf(`(function(t){
			var all = document.querySelectorAll('[role="menuitem"], [role="option"], [data-tid*="MenuItem"], [data-tid*="Popup"] *, li');
			for(var i=0;i<all.length;i++){
				if(all[i].textContent.trim().indexOf(t) !== -1){
					all[i].click();
					return JSON.stringify({ok: true, tag: all[i].tagName});
				}
			}
			var any = document.querySelectorAll('*');
			for(var i=0;i<any.length;i++){
				if(any[i].textContent.trim() === t){
					var el = any[i];
					while(el && el.tagName !== 'BUTTON' && el.tagName !== 'A' && el.tagName !== 'LI'){
						el = el.parentElement;
					}
					if(el){
						el.click();
						return JSON.stringify({ok: true, tag: el.tagName, via: 'walkup'});
					}
				}
			}
			return JSON.stringify({ok: false});
		})(%q)`, text), &result).Do(ctx); err != nil {
			return err
		}
		var r struct {
			Ok  bool   `json:"ok"`
			Tag string `json:"tag"`
		}
		json.Unmarshal([]byte(result), &r)
		if !r.Ok {
			return fmt.Errorf("popup option %q not found", text)
		}
		return nil
	}
}

func SignDeliveryNote(ctx context.Context, number string) error {
	if err := OpenWaybillSidePage(ctx, number); err != nil {
		return fmt.Errorf("open waybill: %w", err)
	}

	if err := chromedp.Run(ctx, clickPopupMenu()); err != nil {
		return fmt.Errorf("open popup menu: %w", err)
	}

	chromedp.Run(ctx, chromedp.Sleep(1*time.Second))

	if err := chromedp.Run(ctx, clickPopupOption("Подписать без подписи водителя")); err != nil {
		return fmt.Errorf("click sign option: %w", err)
	}

	chromedp.Run(ctx, chromedp.Sleep(3*time.Second))

	var posResult string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`
		(function(){
		var certs = document.querySelectorAll('[data-tid^="certificate_"]');
		for(var i=0;i<certs.length;i++){
			if(certs[i].textContent.includes("Сичкарук")){
				var r = certs[i].getBoundingClientRect();
				return JSON.stringify({cx: r.left + r.width/2, cy: r.top + r.height/2, ok: true});
			}
		}
		return JSON.stringify({ok: false});
	})()`, &posResult)); err != nil {
		return err
	}

	var pos struct {
		Ok bool    `json:"ok"`
		CX float64 `json:"cx"`
		CY float64 `json:"cy"`
	}
	json.Unmarshal([]byte(posResult), &pos)
	if !pos.Ok {
		return fmt.Errorf("certificate not found in modal")
	}

	if err := chromedp.Run(ctx,
		chromedp.MouseClickXY(pos.CX, pos.CY),
		chromedp.Sleep(1*time.Second),
	); err != nil {
		return fmt.Errorf("click certificate: %w", err)
	}

	var choosePos string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`
		(function(){
		var btn = document.querySelector('[data-tid="ModalFooter__root"] [data-tid="Button__rootElement"]');
		if(!btn) return JSON.stringify({ok: false, err: 'no button'});
		var r = btn.getBoundingClientRect();
		return JSON.stringify({ok: true, cx: r.left + r.width/2, cy: r.top + r.height/2});
	})()`, &choosePos)); err != nil {
		return err
	}

	var choose struct {
		Ok bool    `json:"ok"`
		CX float64 `json:"cx"`
		CY float64 `json:"cy"`
	}
	json.Unmarshal([]byte(choosePos), &choose)
	if !choose.Ok {
		return fmt.Errorf("choose button not found")
	}

	if err := chromedp.Run(ctx,
		chromedp.MouseClickXY(choose.CX, choose.CY),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return fmt.Errorf("click choose: %w", err)
	}

	var poaResult string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`
		(function(){
		var all = document.querySelectorAll('*');
		var t = 'ОБЩЕСТВО С ОГРАНИЧЕННОЙ ОТВЕТСТВЕННОСТЬЮ СТРОИТЕЛЬНАЯ КОМПАНИЯ "ВОСТОКСПЕЦСТРОЙ';
		for(var i=0;i<all.length;i++){
			if(all[i].textContent.includes(t)){
				all[i].click();
				return 'clicked';
			}
		}
		return 'not found';
	})()`, &poaResult)); err != nil {
		return err
	}
	if poaResult != "clicked" {
		return fmt.Errorf("click power of attorney: %s", poaResult)
	}

	chromedp.Run(ctx, chromedp.Sleep(1*time.Second))

	if err := ClickElement(ctx, "Продолжить"); err != nil {
		return fmt.Errorf("click continue: %w", err)
	}

	return chromedp.Run(ctx, chromedp.Sleep(3*time.Second))
}
