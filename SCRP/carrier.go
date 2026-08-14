package SCRP

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func NavigateToCarrier(ctx context.Context) error {
	var url string
	chromedp.Run(ctx, chromedp.Location(&url))
	log.Printf("NavigateToCarrier: enter url=%s", url)
	takeScreenshot(ctx, "nav_carrier_enter")

	if strings.Contains(url, "/carrier/") {
		log.Println("NavigateToCarrier: already on carrier page")
		return waitForTableRows(ctx)
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
			log.Println("NavigateToCarrier: clicked Перевозчик")
			//takeScreenshot(ctx, "nav_carrier_clicked")
			return waitForTableRows(ctx)
		}
		chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
	}

	takeScreenshot(ctx, "nav_carrier_no_button")
	return fmt.Errorf("Перевозчик button not found after 30s")
}

func waitForTableRows(ctx context.Context) error {
	// Ждём именно реальные строки накладных (с WaybillNumber), а не
	// плейсхолдер "Накладные не найдены", который сам имеет data-tid="TableRow"
	// и заставлял парсить пустую таблицу до загрузки данных.
	for i := 0; i < 30; i++ {
		var n int
		if err := chromedp.Run(ctx, chromedp.Evaluate(
			`document.querySelectorAll('[data-tid="TableRow"] [data-tid="WaybillNumber"]').length`, &n)); err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		// Нет ни одной реальной строки — но, возможно, это честное "пусто".
		// Проверяем признак пустого состояния по всему документу (textContent,
		// не только внутри TableRow — плейсхолдер может быть отдельным блоком).
		var empty bool
		chromedp.Run(ctx, chromedp.Evaluate(
			`(document.body.textContent.indexOf('Накладные не найдены') !== -1)`, &empty))
		if empty {
			return nil
		}
		chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
	}
	// Реальных строк нет и пустого состояния тоже — просто идём дальше
	// (parse вернёт 0 накладных). Не падаем, чтобы Monitor продолжал работу.
	return nil
}

func ParseDeliveryNotes(ctx context.Context) ([]DeliveryNote, error) {
	if err := waitForTableRows(ctx); err != nil {
		return nil, err
	}

	var url string
	chromedp.Run(ctx, chromedp.Location(&url))

	var rowCount int
	chromedp.Run(ctx, chromedp.Evaluate(
		`document.querySelectorAll('[data-tid="TableRow"] [data-tid="WaybillNumber"]').length`, &rowCount))
	log.Printf("ParseDeliveryRows: waybill rows = %d url=%s", rowCount, url)
	takeScreenshot(ctx, "parse_carrier_table")

	// Диагностика: дамп содержимого строк (что за строка без WaybillNumber).
	var dump string
	chromedp.Run(ctx, chromedp.Evaluate(`(function(){
		var rows = document.querySelectorAll('[data-tid="TableRow"]');
		var out = [];
		for(var i=0;i<rows.length;i++){
			out.push('row'+i+': ['+rows[i].innerText.replace(/\s+/g,' ').trim().slice(0,160)+']');
		}
		return out.join(' | ');
	})()`, &dump))
	log.Printf("ParseDeliveryRows dump: %s", dump)

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
	if err != nil {
		log.Printf("ParseDeliveryNotes Evaluate error: %v", err)
	}
	return data, err
}
