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
		return err
	}
	if result == "not found" {
		return fmt.Errorf("element %q not found", text)
	}
	return nil
}

func ElementExists(ctx context.Context, text string) bool {
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
