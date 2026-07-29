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

	for i := 0; i < 30; i++ {
		var result string
		chromedp.Run(ctx, chromedp.Evaluate(`
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
		if result == "clicked" {
			return waitForTableRows(ctx)
		}
		chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
	}

	return fmt.Errorf("Перевозчик button not found after 30s")
}

func waitForTableRows(ctx context.Context) error {
	for i := 0; i < 30; i++ {
		var n int
		if err := chromedp.Run(ctx, chromedp.Evaluate(
			`document.querySelectorAll('[data-tid="TableRow"]').length`, &n)); err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
	}
	return fmt.Errorf("waybill table did not load in 30s")
}

func ParseDeliveryNotes(ctx context.Context) ([]DeliveryNote, error) {
	if err := waitForTableRows(ctx); err != nil {
		return nil, err
	}
	chromedp.Run(ctx, chromedp.Sleep(1*time.Second))

	var data []DeliveryNote
	err := chromedp.Run(ctx, chromedp.Evaluate(`(function(){
		var result = [];
		var rows = document.querySelectorAll('[data-tid="TableRow"]');
		rows.forEach(function(row){
			var text = function(sel){
				var el = row.querySelector(sel);
				return el ? el.textContent.replace(/\s+/g, ' ').trim() : '';
			};
			var cellText = function(cellSel, sel){
				var cell = row.querySelector(cellSel);
				if(!cell) return '';
				var el = cell.querySelector(sel);
				return el ? el.textContent.replace(/\s+/g, ' ').trim() : '';
			};
			result.push({
				number: text('[data-tid="WaybillNumber"]'),
				date: text('[data-tid="WaybillDate"]'),
				consignor: cellText('[data-tid="WaybillSenderCell"]', '[data-tid="Name"]'),
				consignorAddress: cellText('[data-tid="WaybillSenderCell"]', '[data-tid="Address"]'),
				consignee: cellText('[data-tid="WaybillRecipientCell"]', '[data-tid="Name"]'),
				consigneeAddress: cellText('[data-tid="WaybillRecipientCell"]', '[data-tid="Address"]'),
				carrier: text('[data-tid="CarrierName"]'),
				driver: text('[data-tid="DriverName"]'),
				driverPhone: text('[data-tid="DriverPhone"]'),
				truck: text('[data-tid="TruckInfo"]'),
				test: text('[data-tid="TestSign"]')
			});
		});
		return result;
	})()`, &data))
	return data, err
}
