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
	for {
		var posJSON string
		if err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`(function(){
			var el = %s;
			if(!el) return JSON.stringify({ok:false});
			var r = el.getBoundingClientRect();
			if(r.width === 0 || r.height === 0) return JSON.stringify({ok:false});
			return JSON.stringify({ok:true, cx:r.left+r.width/2, cy:r.top+r.height/2});
		})()`, findJS), &posJSON)); err != nil {
			// Если контекст отменён (таймаут) — выходим сразу.
			if ctx.Err() != nil {
				return err
			}
		} else {
			var pos struct {
				Ok bool    `json:"ok"`
				CX float64 `json:"cx"`
				CY float64 `json:"cy"`
			}
			json.Unmarshal([]byte(posJSON), &pos)
			if pos.Ok {
				return chromedp.Run(ctx, chromedp.MouseClickXY(pos.CX, pos.CY))
			}
		}

		if time.Now().After(deadline) {
			break
		}
		chromedp.Run(ctx, chromedp.Sleep(300*time.Millisecond))
	}
	return fmt.Errorf("element not found or invisible (polled %s): %s", pollTimeout, findJS)
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
	if err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`(function(num){
		var rows = document.querySelectorAll('[data-tid="TableRow"]');
		for(var i=0;i<rows.length;i++){
			var numEl = rows[i].querySelector('[data-tid="WaybillNumber"]');
			if(numEl && numEl.textContent.trim() === num){
				var btn = rows[i].querySelector('[data-tid="RowActionsButton"] button[aria-controls*="Popup"], [data-tid="RowActionsButton"] button');
				if(!btn) return 'no menu button';
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
	takeScreenshot(ctx, number+"_2_menu")
	if err := openRowMenu(ctx, number); err != nil {
		return fmt.Errorf("open row menu: %w", err)
	}

	// 2. Пункт "Подписать без подписи водителя"
	signItemJS := `document.querySelector('[data-tid="Popup__root"] [data-tid="SignWithoutDriverSignature"]') !== null`
	if err := waitForJS(ctx, signItemJS, 10*time.Second); err != nil {
		return fmt.Errorf("sign menu item: %w", err)
	}
	takeScreenshot(ctx, number+"_3_sign_item")
	if err := clickAtCenter(ctx, `document.querySelector('[data-tid="SignWithoutDriverSignature"]')`); err != nil {
		return fmt.Errorf("click sign item: %w", err)
	}

	// 3. SidePage "Подписание накладной" -> кнопка "Подписать" в футере
	sidePageJS := `document.querySelector('[data-tid="SidePageFooter__root"] [data-tid="Sign"]') !== null`
	if err := waitForJS(ctx, sidePageJS, 15*time.Second); err != nil {
		return fmt.Errorf("sign side page: %w", err)
	}
	takeScreenshot(ctx, number+"_4_side_page")
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
	if err := waitForJS(ctx, `document.querySelector('[data-tid^="certificate_"]') !== null`, 30*time.Second); err != nil {
		return fmt.Errorf("certificate modal: %w", err)
	}
	takeScreenshot(ctx, number+"_5_cert")
	certJS := fmt.Sprintf(`(function(user){
		var certs = document.querySelectorAll('[data-tid^="certificate_"]');
		for(var i=0;i<certs.length;i++){
			if(certs[i].textContent.indexOf(user) !== -1) return certs[i];
		}
		return null;
	})(%q)`, certUser)
	if err := clickAtCenter(ctx, certJS); err != nil {
		return fmt.Errorf("click certificate: %w", err)
	}
	chromedp.Run(ctx, chromedp.Sleep(500*time.Millisecond))
	chooseJS := `(function(){
		var c = document.querySelector('[data-tid="ModalFooter__root"] [data-tid="Choose"], [data-tid="Choose"]');
		return c ? (c.tagName === 'BUTTON' ? c : (c.querySelector('button') || c)) : null;
	})()`
	if err := clickAtCenter(ctx, chooseJS); err != nil {
		return fmt.Errorf("click Choose: %w", err)
	}

	// 5. Модалка "Выбор доверенности" -> доверенность -> "Продолжить"
	if err := waitForJS(ctx, `document.querySelector('[data-tid="representative-poa-list-item"]') !== null`, 30*time.Second); err != nil {
		return fmt.Errorf("poa modal: %w", err)
	}
	takeScreenshot(ctx, number+"_6_poa")
	if err := clickAtCenter(ctx, `document.querySelector('[data-tid="representative-poa-list-item"]')`); err != nil {
		return fmt.Errorf("click poa: %w", err)
	}
	chromedp.Run(ctx, chromedp.Sleep(500*time.Millisecond))
	continueJS := `(function(){
		var b = document.querySelector('[data-tid="continue-with-poa-button"]');
		return b ? (b.tagName === 'BUTTON' ? b : (b.querySelector('button') || b)) : null;
	})()`
	if err := clickAtCenter(ctx, continueJS); err != nil {
		return fmt.Errorf("click continue: %w", err)
	}

	// 6. Ждём закрытия модалки — подписание выполняется криптоплагином
	takeScreenshot(ctx, number+"_7_signing")
	if err := waitForJS(ctx,
		`document.querySelector('[data-tid*="Modal"][data-tid*="root"], [role="dialog"]') === null`,
		90*time.Second); err != nil {
		return fmt.Errorf("signing did not finish: %w", err)
	}

	takeScreenshot(ctx, number+"_8_done")
	chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
	return nil
}
