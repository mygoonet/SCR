package SCRP

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// StartWebServer запускает HTTP-сервер со списком накладных (порт cfg.WebAddr).
//
//	GET /            — HTML-страница: список накладных (screenshots/<номер>/),
//	                    статус, превью всех скриншотов от логина до подписи.
//	GET /screenshots — статические файлы папок (FileServer).
func StartWebServer(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.Handle("/screenshots/", http.StripPrefix("/screenshots/", http.FileServer(http.Dir(screenshotDir))))

	log.Printf("Web: список накладных на http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("Web server error: %v", err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	entries, err := os.ReadDir(screenshotDir)
	if err != nil {
		fmt.Fprintf(w, "ошибка чтения %s: %v", screenshotDir, err)
		return
	}

	type noteItem struct {
		num     string
		created time.Time
	}
	items := make([]noteItem, 0, len(entries))
	const timeLayout = "2006-01-02 15:04:05"
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		num := e.Name()
		dir := filepath.Join(screenshotDir, num)
		created := time.Time{}
		if b, err := os.ReadFile(filepath.Join(dir, "note.json")); err == nil {
			var nf NoteFileJSON
			if json.Unmarshal(b, &nf) == nil && nf.CreatedAt != "" {
				if t, err := time.Parse(timeLayout, nf.CreatedAt); err == nil {
					created = t
				}
			}
		}
		items = append(items, noteItem{num: num, created: created})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].created.After(items[j].created)
	})

	fmt.Fprint(w, "<html><head><meta charset='utf-8'><title>Накладные</title></head><body>")
	fmt.Fprintf(w, "<h1>Накладные (%d)</h1>", len(items))

	if len(items) == 0 {
		fmt.Fprint(w, "<p>нет данных</p></body></html>")
		return
	}

	// Подсчёт ошибок и проверка времени последнего fetch
	errorCount := 0
	var lastCreated time.Time
	if len(items) > 0 {
		lastCreated = items[0].created
	}
	for _, it := range items {
		dir := filepath.Join(screenshotDir, it.num)
		if b, err := os.ReadFile(filepath.Join(dir, "note.json")); err == nil {
			var nf NoteFileJSON
			if json.Unmarshal(b, &nf) == nil {
				if nf.Error != "" || nf.Status == "failed" {
					errorCount++
				}
			}
		}
	}
	if errorCount > 0 {
		fmt.Fprintf(w, "<p style='color:red'>Ошибки по накладным: %d</p>", errorCount)
	}
	if !lastCreated.IsZero() {
		if time.Since(lastCreated) > 2*time.Hour {
			fmt.Fprintf(w, "<p style='color:orange'>Внимание: последнее обновление накладных %s (более 2 часов назад)</p>", lastCreated.Format(timeLayout))
		} else {
			fmt.Fprintf(w, "<p>Последнее обновление: %s</p>", lastCreated.Format(timeLayout))
		}
	}

	fmt.Fprint(w, `<table border="1" cellpadding="6" cellspacing="0" style="border-collapse:collapse">
	<tr><th>Номер</th><th>Создано</th><th>Дата</th><th>Отправитель → Получатель</th><th>Статус</th><th>Ошибка</th><th>Скриншоты</th></tr>`)

	for _, it := range items {
		num := it.num
		dir := filepath.Join(screenshotDir, num)
		var nf NoteFileJSON
		if b, err := os.ReadFile(filepath.Join(dir, "note.json")); err == nil {
			_ = json.Unmarshal(b, &nf)
		}

		fmt.Fprintf(w, "<tr><td><b>%s</b></td>", html.EscapeString(num))
		fmt.Fprintf(w, "<td>%s</td>", html.EscapeString(nf.CreatedAt))
		fmt.Fprintf(w, "<td>%s</td>", html.EscapeString(nf.Date))
		fmt.Fprintf(w, "<td>%s → %s</td>",
			html.EscapeString(nf.Consignor), html.EscapeString(nf.Consignee))
		fmt.Fprintf(w, "<td>%s</td>", html.EscapeString(nf.Status))
		errMsg := html.EscapeString(nf.Error)
		if errMsg == "" && nf.Status == "failed" {
			errMsg = "failed"
		}
		if errMsg != "" {
			fmt.Fprintf(w, "<td style='color:red'>%s</td>", errMsg)
		} else {
			fmt.Fprint(w, "<td></td>")
		}

		// Превью всех скриншотов папки (от логина до подписи).
		fmt.Fprint(w, "<td>")
		files, _ := os.ReadDir(dir)
		names := make([]string, 0, len(files))
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".png") {
				names = append(names, f.Name())
			}
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(w, `<a href="/screenshots/%s/%s"><img src="/screenshots/%s/%s" width="90"></a> `,
				html.EscapeString(num), html.EscapeString(n), html.EscapeString(num), html.EscapeString(n))
		}
		fmt.Fprint(w, "</td></tr>")
	}

	fmt.Fprint(w, "</table>")

	// Блок критических ошибок и информации о тикере
	fmt.Fprint(w, "<h2>Критические ошибки</h2>")
	lastFetchMu.Lock()
	fetchErr := lastFetchError
	fails := make([]string, len(signingFailures))
	copy(fails, signingFailures)
	lastFetchMu.Unlock()

	criticalItems := []string{}
	if fetchErr != "" {
		criticalItems = append(criticalItems, "Ошибка получения накладных: "+fetchErr)
	}
	criticalItems = append(criticalItems, fails...)
	if len(criticalItems) > 0 {
		fmt.Fprint(w, "<ul style='color:red'>")
		for _, e := range criticalItems {
			fmt.Fprintf(w, "<li>%s</li>", html.EscapeString(e))
		}
		fmt.Fprint(w, "</ul>")
	} else {
		fmt.Fprint(w, "<p>Критических ошибок нет</p>")
	}

	now := time.Now()

	// Читаем состояние последнего тикера
	lastFetchMu.Lock()
	tickerTime := lastFetchTime
	notesCopy := make([]DeliveryNote, len(lastNotes))
	copy(notesCopy, lastNotes)
	lastFetchMu.Unlock()

	fmt.Fprintf(w, "<h3>Время обновления тикера</h3>")
	if tickerTime.IsZero() {
		fmt.Fprint(w, "<p>Нет данных о последнем обновлении тикера</p>")
	} else {
		diff := now.Sub(tickerTime)
		roundedMin := int(diff.Minutes())
		if diff.Minutes()-float64(roundedMin) >= 0.5 {
			roundedMin++
		}
		fmt.Fprintf(w, "<p>Последнее обновление тикера: %s</p>", tickerTime.Format("2006/01/02 15:04:05"))
		fmt.Fprintf(w, "<p>Текущее время: %s</p>", now.Format("2006/01/02 15:04:05"))
		fmt.Fprintf(w, "<p><b>Прошло с последнего успешного тикера: %d мин</b></p>", roundedMin)
	}

	fmt.Fprintf(w, "<h3>Список накладных из последнего тикера (%d)</h3><pre>", len(notesCopy))
	if len(notesCopy) == 0 {
		fmt.Fprint(w, "Нет накладных в последнем тикере\n")
	} else {
		for _, n := range notesCopy {
			line := fmt.Sprintf("%s  от %s  %s → %s  %s",
				n.Number,
				n.Date,
				html.EscapeString(n.Consignor),
				html.EscapeString(n.Consignee),
				html.EscapeString(n.Carrier))
			fmt.Fprintf(w, "%s\n", line)
		}
	}
	fmt.Fprint(w, "</pre></body></html>")
}
