package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── Глобальное хранилище ошибок ─────────────────────────────────────────────

type ErrorEntry struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // "fetch", "sign", "browser", "api"
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Count     int       `json:"count"`
	Resolved  bool      `json:"resolved"`
}

var (
	errorsMu sync.Mutex
	errors   = make(map[string]ErrorEntry)
)

func AddError(errType, message string) {
	errorsMu.Lock()
	defer errorsMu.Unlock()

	id := fmt.Sprintf("%s-%d", errType, time.Now().Unix())
	errors[id] = ErrorEntry{
		ID:        id,
		Type:      errType,
		Message:   message,
		Timestamp: time.Now(),
		Count:     1,
		Resolved:  false,
	}
}

func GetErrors() []ErrorEntry {
	errorsMu.Lock()
	defer errorsMu.Unlock()

	result := make([]ErrorEntry, 0, len(errors))
	for _, e := range errors {
		result = append(result, e)
	}
	return result
}

func ClearErrors() {
	errorsMu.Lock()
	defer errorsMu.Unlock()
	errors = make(map[string]ErrorEntry)
}

// ─── HTTP сервер ──────────────────────────────────────────────────────────────

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	errors := GetErrors()
	unresolved := 0
	typeCounts := make(map[string]int)

	for _, e := range errors {
		if !e.Resolved {
			unresolved++
		}
		typeCounts[e.Type]++
	}

	fmt.Fprintf(w, `<html><head><meta charset="utf-8"><title>DevTools — Ошибки</title>
<style>
body { font-family: monospace; background: #1a1a2e; color: #eee; margin: 0; padding: 20px; }
h1 { color: #e94560; }
.stats { display: flex; gap: 20px; margin: 20px 0; }
.stat { background: #16213e; padding: 15px; border-radius: 8px; min-width: 150px; text-align: center; }
.stat .num { font-size: 2em; font-weight: bold; }
.stat.errors .num { color: #e94560; }
.stat.resolved .num { color: #0f3460; }
.error-list { background: #16213e; padding: 15px; border-radius: 8px; margin-top: 20px; }
.error-item { background: #0f3460; padding: 10px; margin: 10px 0; border-radius: 4px; border-left: 4px solid #e94560; }
.error-item.resolved { border-left-color: #4ecca3; opacity: 0.6; }
.badge { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 0.8em; margin-right: 10px; }
.badge.fetch { background: #e94560; }
.badge.sign { background: #f39c12; }
.badge.browser { background: #3498db; }
.badge.api { background: #9b59b6; }
button { background: #e94560; color: white; border: none; padding: 10px 20px; border-radius: 4px; cursor: pointer; margin: 10px 0; }
button:hover { background: #c0392b; }
.time { color: #888; font-size: 0.9em; }
</style></head><body>`)

	fmt.Fprintf(w, `<h1>🔧 DevTools — Мониторинг ошибок</h1>`)
	fmt.Fprintf(w, `<div class="stats">`)
	fmt.Fprintf(w, `<div class="stat errors"><div class="num">%d</div><div>Всего ошибок</div></div>`, len(errors))
	fmt.Fprintf(w, `<div class="stat errors"><div class="num">%d</div><div>Активных</div></div>`, unresolved)
	fmt.Fprintf(w, `<div class="stat resolved"><div class="num">%d</div><div>Разрешено</div></div>`, len(errors)-unresolved)
	fmt.Fprintf(w, `</div>`)

	fmt.Fprint(w, `<button onclick="location.reload()">🔄 Обновить</button>`)
	fmt.Fprint(w, `<button onclick="clearErrors()">🗑 Очистить всё</button>`)

	fmt.Fprint(w, `<div class="error-list"><h2>Список ошибок</h2>`)
	if len(errors) == 0 {
		fmt.Fprint(w, `<p style="color: #4ecca3;">✅ Критических ошибок нет</p>`)
	} else {
		for _, e := range errors {
			resolvedClass := ""
			if e.Resolved {
				resolvedClass = " resolved"
			}
			fmt.Fprintf(w, `<div class="error-item%s">`, resolvedClass)
			fmt.Fprintf(w, `<span class="badge badge-%s">%s</span>`, e.Type, e.Type)
			fmt.Fprintf(w, `<span class="time">%s</span> `, e.Timestamp.Format("2006-01-02 15:04:05"))
			fmt.Fprintf(w, `<div>%s</div>`, e.Message)
			fmt.Fprintf(w, `<div class="time">ID: %s | Count: %d</div>`, e.ID, e.Count)
			fmt.Fprint(w, `</div>`)
		}
	}
	fmt.Fprint(w, `</div>`)

	fmt.Fprint(w, `<script>
function clearErrors() {
	if(confirm('Очистить все ошибки?')) {
		fetch('/api/clear', {method: 'POST'});
		location.reload();
	}
}
setInterval(() => fetch('/api/errors').then(r => r.json()).then(data => {
	const badge = document.querySelector('.stat.errors .num');
	if(badge) badge.textContent = data.length;
}), 5000);
</script></body></html>`)
}

func handleAPIErrors(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	errors := GetErrors()
	json.NewEncoder(w).Encode(errors)
}

func handleAPIClear(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ClearErrors()
	json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
}

// ─── Мониторинг директории скриншотов ────────────────────────────────────────

func monitorScreenshots(screenshotDir string, stopCh chan struct{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			entries, err := os.ReadDir(screenshotDir)
			if err != nil {
				AddError("browser", fmt.Sprintf("Ошибка чтения скриншотов: %v", err))
				continue
			}

			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				dir := filepath.Join(screenshotDir, e.Name())
				noteFile := filepath.Join(dir, "note.json")
				if b, err := os.ReadFile(noteFile); err == nil {
					var data map[string]interface{}
					if json.Unmarshal(b, &data) == nil {
						if status, ok := data["status"].(string); ok && status == "failed" {
							if errMsg, ok := data["error"].(string); ok && errMsg != "" {
								AddError("sign", fmt.Sprintf("Ошибка подписи %s: %s", e.Name(), errMsg))
							}
						}
					}
				}
			}
		case <-stopCh:
			return
		}
	}
}

// ─── Main ────────────────────────────────────────────────────────────────────

func main() {
	screenshotDir := os.Getenv("SCREENSHOT_DIR")
	if screenshotDir == "" {
		screenshotDir = "./screenshots"
	}

	addr := ":9090"
	log.Printf("DevTools starting on %s", addr)
	log.Printf("Dashboard: http://localhost%s", addr)
	log.Printf("API:       http://localhost%s/api/errors", addr)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleDashboard)
	mux.HandleFunc("/api/errors", handleAPIErrors)
	mux.HandleFunc("/api/clear", handleAPIClear)

	go func() {
		stopCh := make(chan struct{})
		defer close(stopCh)
		monitorScreenshots(screenshotDir, stopCh)
	}()

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("DevTools server error: %v", err)
	}
}
