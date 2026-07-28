package SCRP

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func SignDeliveryNote(ctx context.Context, number string) error {
	fmt.Println("=== SIGN: set viewport ===")
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1920, 1080)); err != nil {
		return fmt.Errorf("set viewport: %w", err)
	}

	fmt.Println("=== SIGN: navigate to login ===")
	if err := NavigateToLogin(ctx); err != nil {
		return fmt.Errorf("navigate to login: %w", err)
	}
	fmt.Println("=== SIGN: navigate to carrier ===")
	if err := NavigateToCarrier(ctx); err != nil {
		return fmt.Errorf("navigate to carrier: %w", err)
	}

	fmt.Println("=== SIGN: open three-dot menu ===")
	if err := openThreeDotMenu(ctx, number); err != nil {
		return fmt.Errorf("open menu: %w", err)
	}

	fmt.Println("=== SIGN: click 'Подписать без подписи водителя' ===")
	if err := ClickElement(ctx, "Подписать без подписи водителя"); err != nil {
		return fmt.Errorf("click sign without driver: %w", err)
	}

	chromedp.Run(ctx, chromedp.Sleep(3*time.Second))
	fmt.Println("=== SIGN: click 'Подписать' in footer ===")
	if err := chromedp.Run(ctx,
		chromedp.Click(`[data-tid="SidePageFooter__root"] [data-tid="Button__rootElement"]`, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
	); err != nil {
		return fmt.Errorf("click sign in footer: %w", err)
	}

	fmt.Println("=== SIGN: select certificate ===")
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

	fmt.Println("=== SIGN: click 'Выбрать' ===")
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

	var pageText string
	chromedp.Run(ctx, chromedp.Evaluate(
		`document.body.innerText.substring(0, 5000)`, &pageText))
	fmt.Println("=== PAGE AFTER CERT CHOOSE ===")
	fmt.Println(pageText)
	fmt.Println("=")

	fmt.Println("=== SIGN: select power of attorney ===")
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

	fmt.Println("=== SIGN: signing completed ===")
	return chromedp.Run(ctx, chromedp.Sleep(3*time.Second))
}

func openThreeDotMenu(ctx context.Context, number string) error {
	if err := chromedp.Run(ctx, chromedp.WaitVisible(`table`, chromedp.ByQuery), chromedp.Sleep(3*time.Second)); err != nil {
		return fmt.Errorf("wait for table: %w", err)
	}

	fmt.Println("=== THREE-DOT: searching for waybill", number, "===")
	var result string
	err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(
		`(function(num){
		var rows = document.querySelectorAll('table tbody tr');
		for(var i=0;i<rows.length;i++){
			var cells = rows[i].querySelectorAll('td');
			for(var j=0;j<cells.length;j++){
				if(cells[j].textContent.indexOf(num) >= 0){
					var btn = rows[i].querySelector('[data-tid="Button__rootElement"]');
					if(!btn) return JSON.stringify({err:'no Button__rootElement'});
					btn.setAttribute('aria-expanded', 'true');
					btn.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true, view: window}));
					return JSON.stringify({ok: true});
				}
			}
		}
		return JSON.stringify({err: 'not found'});
	})(%q)`, number), &result))
	if err != nil {
		return err
	}
	fmt.Println("=== THREE-DOT: result:", result, "===")
	if !strings.Contains(result, `"ok"`) {
		return fmt.Errorf("openThreeDotMenu: %s", result)
	}
	return chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
}
