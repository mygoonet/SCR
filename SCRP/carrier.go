package SCRP

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func NavigateToCarrier(ctx context.Context) error {
	var url string
	chromedp.Run(ctx, chromedp.Location(&url))

	if strings.Contains(url, "/carrier/") {
		return nil
	}

	var result string
	err := chromedp.Run(ctx, chromedp.Evaluate(`
		(function(){
			var btns = document.querySelectorAll('[data-tid="OrgItemRoleButton"] button');
			for(var i=0;i<btns.length;i++){
				if(btns[i].textContent.trim() === 'Перевозчик'){
					btns[i].click();
					return 'clicked';
				}
			}
			return 'not found';
		})()`, &result))
	if err != nil {
		return fmt.Errorf("eval click: %w", err)
	}
	if result != "clicked" {
		return fmt.Errorf("Перевозчик button not found")
	}

	return chromedp.Run(ctx,
		chromedp.Sleep(2*time.Second),
		chromedp.WaitVisible(`[data-tid="ListItem"]`, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
	)
}

func ParseDeliveryNotes(ctx context.Context) ([]DeliveryNote, error) {
	chromedp.Run(ctx,
		chromedp.Sleep(2*time.Second),
		chromedp.WaitVisible(`[data-tid="ListItem"]`, chromedp.ByQuery),
	)

	var data []DeliveryNote
	err := chromedp.Run(ctx, chromedp.Evaluate(`(function(){
		var result = [];
		var items = document.querySelectorAll('[data-tid="ListItem"]');
		items.forEach(function(item){
			var number = (item.querySelector('[data-tid="WaybillNumber"]') || {}).textContent || '';
			var consignor = (item.querySelector('[data-tid="ConsignorName"]') || {}).textContent || '';
			var consignee = (item.querySelector('[data-tid="ConsigneeName"]') || {}).textContent || '';
			var address = (item.querySelector('[data-tid="Address"]') || {}).textContent || '';
			var cargo = (item.querySelector('[data-tid="CargoValuesCount"]') || {}).textContent || '';
			var desc = (item.querySelector('[data-tid="CargoDescription"]') || {}).textContent || '';
			var cat = '';
			var container = item.closest('[data-tid="TransportationListCategoryHeader"]');
			if(!container){
				var prev = item.parentElement.previousElementSibling;
				if(prev && prev.getAttribute('data-tid') === 'ListHeader') cat = prev.textContent || '';
			}
			var header = item.parentElement.parentElement.querySelector('[data-tid="ListHeader"]');
			if(header) cat = header.textContent || '';

			result.push({
				number: number.trim(),
				consignor: consignor.trim(),
				consignee: consignee.trim(),
				address: address.trim(),
				cargo: cargo.replace(/\s+/g, ' ').trim(),
				description: desc.trim(),
				category: cat.trim()
			});
		});
		return result;
	})()`, &data))
	return data, err
}
