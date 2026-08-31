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

// StartWebServer запускает HTTP-сервер (порт cfg.WebAddr).
//
//	GET /              — legacy HTML (обратная совместимость)
//	GET /vue           — Vue SPA (frontend/dist)
//	GET /vue/*         — SPA fallback -> index.html
//	GET /assets/*      — статика Vue (из frontend/dist/assets)
//	GET /screenshots/* — скриншоты
//	GET /api/notes     — JSON список накладных (обновляется каждые 5с на фронте)
//	GET /api/status    — JSON статус тикера/ошибок
func StartWebServer(addr string) {
	mux := http.NewServeMux()

	// legacy
	mux.HandleFunc("/", handleIndex)

	// api
	mux.HandleFunc("/api/notes", handleAPINotes)
	mux.HandleFunc("/api/status", handleAPIStatus)

	// vue spa
	mux.HandleFunc("/vue", handleVue)
	mux.HandleFunc("/vue/", handleVue)

	// screenshots
	mux.Handle("/screenshots/", http.StripPrefix("/screenshots/", http.FileServer(http.Dir(screenshotDir))))

	// попытка отдать /assets напрямую (Vite build кладёт туда js/css)
	for _, dir := range vueDistDirs() {
		assetsDir := filepath.Join(dir, "assets")
		if st, err := os.Stat(assetsDir); err == nil && st.IsDir() {
			mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(assetsDir))))
			log.Printf("Web: Vue assets from %s", assetsDir)
			break
		}
	}

	log.Printf("Web: legacy  http://localhost%s/", addr)
	log.Printf("Web: vue     http://localhost%s/vue", addr)
	log.Printf("Web: api     http://localhost%s/api/notes", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("Web server error: %v", err)
	}
}

// ─── Vue SPA ────────────────────────────────────────────────────────────────

func vueDistDirs() []string {
	return []string{
		"frontend/dist",
		"./frontend/dist",
		"/frontend/dist",
		"dist",
		"./dist",
		filepath.Join(filepath.Dir(os.Args[0]), "frontend/dist"),
	}
}

func findVueDist() string {
	for _, d := range vueDistDirs() {
		if st, err := os.Stat(filepath.Join(d, "index.html")); err == nil && !st.IsDir() {
			return d
		}
	}
	return ""
}

func handleVue(w http.ResponseWriter, r *http.Request) {
	dist := findVueDist()
	if dist == "" {
		http.Error(w, "Vue dist not found. Run: cd frontend && npm run build", http.StatusNotFound)
		return
	}
	// /vue -> /vue/ redirect для корректных относительных путей
	if r.URL.Path == "/vue" {
		http.Redirect(w, r, "/vue/", http.StatusMovedPermanently)
		return
	}
	// strip /vue/ prefix, serve from dist
	path := strings.TrimPrefix(r.URL.Path, "/vue/")
	if path == "" {
		path = "index.html"
	}
	full := filepath.Join(dist, filepath.FromSlash(path))

	// SPA fallback: если файла нет — отдаём index.html
	if _, err := os.Stat(full); os.IsNotExist(err) {
		http.ServeFile(w, r, filepath.Join(dist, "index.html"))
		return
	}
	http.ServeFile(w, r, full)
}

// ─── API ───────────────────────────────────────────────────────────────────

func handleAPINotes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")

	entries, err := os.ReadDir(screenshotDir)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	type apiNote struct {
		NoteFileJSON
		Number string   `json:"number"`
		Shots  []string `json:"shots"`
	}
	// собираем как в handleIndex, сортируем по created
	type tmp struct {
		num     string
		created time.Time
		nf      NoteFileJSON
		shots   []string
	}
	var tmpList []tmp
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		num := e.Name()
		dir := filepath.Join(screenshotDir, num)
		var nf NoteFileJSON
		if b, err := os.ReadFile(filepath.Join(dir, "note.json")); err == nil {
			_ = json.Unmarshal(b, &nf)
		}
		created := time.Time{}
		for _, s := range []string{nf.CreatedAt, nf.ProcessedAt} {
			if t, err := time.Parse(timeLayout, s); err == nil {
				created = t
				break
			}
		}
		// скриншоты
		var shots []string
		if files, _ := os.ReadDir(dir); len(files) > 0 {
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".png") {
					shots = append(shots, f.Name())
				}
			}
			sort.Strings(shots)
		}
		tmpList = append(tmpList, tmp{num: num, created: created, nf: nf, shots: shots})
	}
	sort.Slice(tmpList, func(i, j int) bool { return tmpList[i].created.After(tmpList[j].created) })

	out := make([]apiNote, 0, len(tmpList))
	for _, t := range tmpList {
		out = append(out, apiNote{NoteFileJSON: t.nf, Number: t.num, Shots: t.shots})
	}
	json.NewEncoder(w).Encode(out)
}

func handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")

	lastFetchMu.Lock()
	tickerTime := lastFetchTime
	notesCopy := append([]DeliveryNote(nil), lastNotes...)
	fetchErr := lastFetchError
	fails := append([]string(nil), signingFailures...)
	lastFetchMu.Unlock()

	resp := map[string]interface{}{
		"now":               time.Now().Format(timeLayout),
		"lastFetchTime":     "",
		"minutesSinceFetch": nil,
		"lastFetchError":    fetchErr,
		"signingFailures":   fails,
		"lastNotes":         notesCopy,
		"lastNotesCount":    len(notesCopy),
	}
	if !tickerTime.IsZero() {
		resp["lastFetchTime"] = tickerTime.Format(timeLayout)
		diff := time.Since(tickerTime)
		mins := int(diff.Minutes() + 0.5)
		resp["minutesSinceFetch"] = mins
	}
	json.NewEncoder(w).Encode(resp)
}

// ─── Legacy HTML (обратная совместимость) ──────────────────────────────────

func handleIndex(w http.ResponseWriter, r *http.Request) {
	// /vue* и /api* и /assets* и /screenshots* уже обработаны выше,
	// но "/" матчит всё — отдаём 404 для неизвестных путей кроме "/"
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
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		num := e.Name()
		dir := filepath.Join(screenshotDir, num)
		created := time.Time{}
		if b, err := os.ReadFile(filepath.Join(dir, "note.json")); err == nil {
			var nf NoteFileJSON
			if json.Unmarshal(b, &nf) == nil {
				for _, s := range []string{nf.CreatedAt, nf.ProcessedAt} {
					if t, err := time.Parse(timeLayout, s); err == nil {
						created = t
						break
					}
				}
			}
		}
		items = append(items, noteItem{num: num, created: created})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].created.After(items[j].created)
	})

	fmt.Fprint(w, "<html><head><meta charset='utf-8'><title>Накладные</title></head><body>")
	// ссылка на новую Vue-версию
	fmt.Fprint(w, `<p><a href="/vue/">→ Новая версия (Vue)</a></p>`)
	fmt.Fprintf(w, "<h1>Накладные (%d)</h1>", len(items))

	if len(items) == 0 {
		fmt.Fprint(w, "<p>нет данных</p></body></html>")
		return
	}

	fmt.Fprint(w, `<table border="1" cellpadding="6" cellspacing="0" style="border-collapse:collapse">
	<tr><th>Номер</th><th>Создано</th><th>Signed</th><th>Дата</th><th>Отправитель → Получатель</th><th>Статус</th><th>Ошибка</th><th>Скриншоты</th></tr>`)

	for _, it := range items {
		num := it.num
		dir := filepath.Join(screenshotDir, num)
		var nf NoteFileJSON
		if b, err := os.ReadFile(filepath.Join(dir, "note.json")); err == nil {
			_ = json.Unmarshal(b, &nf)
		}

		fmt.Fprintf(w, "<tr><td><b>%s</b></td>", html.EscapeString(num))
		fmt.Fprintf(w, "<td>%s</td>", html.EscapeString(nf.CreatedAt))
		signed := strings.Join(nf.SignedAt, ", ")
		if signed == "" {
			signed = nf.ProcessedAt
		}
		fmt.Fprintf(w, "<td>%s</td>", html.EscapeString(signed))
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
