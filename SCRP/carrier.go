package SCRP

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

func NavigateToCarrier(ctx context.Context) error {
	if err := ClickElement(ctx, "Перевозчик"); err != nil {
		return fmt.Errorf("click Перевозчик: %w", err)
	}
	return chromedp.Run(ctx, chromedp.Sleep(3*time.Second))
}

func ParseDeliveryNotes(ctx context.Context) ([]DeliveryNote, error) {
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
