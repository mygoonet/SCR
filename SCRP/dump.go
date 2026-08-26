package SCRP

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
)

// DumpTransportationsRaw перехватывает ответ SPA /transportations и сохраняет
// СЫРОЙ JSON (без парсинга в DeliveryNote) в файл outPath. Ничего не подписывает:
// логинится, перезагружает список, перехватывает тело ответа и закрывает сессию.
// Возвращает путь к файлу.
func DumpTransportationsRaw(browser *Browser, cfg Config, outPath string, timeoutSec int) (string, error) {
	session := browser.NewSession()
	defer session.Close()
	ctx := session.Ctx()

	if err := initSession(ctx, cfg); err != nil {
		return "", fmt.Errorf("init session: %w", err)
	}

	cs := startTransportationsCapture(ctx)

	// Отпускаем старые паузы и перезагружаем страницу для свежего запроса.
	continuePaused(ctx, cs)
	if err := reloadNotesAPI(ctx, cs); err != nil {
		return "", fmt.Errorf("reload: %w", err)
	}

	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	var listID fetch.RequestID
	for listID == "" && time.Now().Before(deadline) {
		if ctx.Err() != nil {
			continuePaused(ctx, cs)
			return "", fmt.Errorf("context cancelled: %w", ctx.Err())
		}
		if id, ok := cs.newestListRequest(); ok {
			listID = id
		}
		if listID == "" {
			time.Sleep(400 * time.Millisecond)
		}
	}
	if listID == "" {
		continuePaused(ctx, cs)
		return "", fmt.Errorf("не перехвачен запрос /transportations за %ds", timeoutSec)
	}

	var raw []byte
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		b, err := fetch.GetResponseBody(listID).Do(c)
		if err != nil {
			return err
		}
		raw = b
		return nil
	})); err != nil {
		continuePaused(ctx, cs)
		return "", fmt.Errorf("get response body: %w", err)
	}
	continuePaused(ctx, cs)

	if err := os.WriteFile(outPath, raw, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	log.Printf("DumpTransportationsRaw: %d байт в %s", len(raw), outPath)
	return outPath, nil
}
