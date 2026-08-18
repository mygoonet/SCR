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

	var nums []string
	for _, e := range entries {
		if e.IsDir() {
			nums = append(nums, e.Name())
		}
	}
	sort.Strings(nums)

	fmt.Fprint(w, "<html><head><meta charset='utf-8'><title>Накладные</title></head><body>")
	fmt.Fprintf(w, "<h1>Накладные (%d)</h1>", len(nums))

	if len(nums) == 0 {
		fmt.Fprint(w, "<p>нет данных</p></body></html>")
		return
	}

	fmt.Fprint(w, `<table border="1" cellpadding="6" cellspacing="0" style="border-collapse:collapse">
	<tr><th>Номер</th><th>Создано</th><th>Дата</th><th>Отправитель → Получатель</th><th>Статус</th><th>Скриншоты</th></tr>`)

	for _, num := range nums {
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

	fmt.Fprint(w, "</table></body></html>")
}
