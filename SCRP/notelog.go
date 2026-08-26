package SCRP

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const timeLayout = "2006-01-02 15:04:05"

// flowShots — буфер «фоновых» скриншотов сессии (логин/навигация/таблица),
// сделанных ДО начала подписания. Когда для накладной создаётся NoteLogger,
// весь буфер копируется в её папку — так в каждой накладной окажутся все
// скриншоты от логина до подписи.
var (
	flowMu    sync.Mutex
	flowShots []flowShot
)

type flowShot struct {
	name string
	data []byte
}

func captureFlowShot(name string, data []byte) {
	flowMu.Lock()
	defer flowMu.Unlock()
	flowShots = append(flowShots, flowShot{name: name, data: data})
}

func currentFlowShots() []flowShot {
	flowMu.Lock()
	defer flowMu.Unlock()
	// возвращаем копию среза, чтобы внешний код не мог модифицировать глобальный буфер
	return append([]flowShot(nil), flowShots...)
}

// activeLogger — логгер накладной, которая сейчас подписывается (одна в момент
// времени, подписание идёт последовательно). Пока он активен, takeScreenshot
// пишет скриншоты в его папку (шаги подписания), а не в буфер сессии.
var (
	activeMu sync.Mutex
	activeNL *NoteLogger
)

func setActiveLogger(l *NoteLogger) {
	activeMu.Lock()
	defer activeMu.Unlock()
	activeNL = l
}

func getActiveLogger() *NoteLogger {
	activeMu.Lock()
	defer activeMu.Unlock()
	return activeNL
}

// NoteLogger ведёт папку screenshots/<number>/ для одной накладной:
//
//	note.json   — данные накладной + status (in_progress/signed/failed)
//	note.txt    — текстовый лог (append-only, никогда не сбрасывается)
//	01_*.png... — все скриншоты сессии (копия буфера + шаги подписания)
type NoteLogger struct {
	Number string
	Dir    string
	note   DeliveryNote
	file   *os.File
	mu     sync.Mutex
	order  int
}

// NoteFileJSON — формат note.json на диске: данные накладной + статус.
// CreatedAt (из DeliveryNote) — реальное время создания документа из API;
// ProcessedAt — когда бот впервые начал её обрабатывать.
type NoteFileJSON struct {
	DeliveryNote
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	ProcessedAt string `json:"processedAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func NewNoteLogger(n DeliveryNote) *NoteLogger {
	dir := filepath.Join(screenshotDir, n.Number)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("NoteLogger %s: mkdir: %v", n.Number, err)
	}

	l := &NoteLogger{Number: n.Number, Dir: dir, note: n}
	l.writeNoteJSON(n, "in_progress", "")

	// note.txt — только дозапись, лог накладной не сбрасывается между запусками.
	f, err := os.OpenFile(filepath.Join(dir, "note.txt"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("NoteLogger %s: open note.txt: %v", n.Number, err)
	} else {
		l.file = f
	}

	// Копируем фоновые скриншоты сессии (логин/nav/таблица) в папку накладной.
	for _, s := range currentFlowShots() {
		l.saveScreenshot(s.name, s.data)
	}

	return l
}

// Logf пишет строку в общий лог и дописывает в note.txt накладной.
func (l *NoteLogger) Logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[накладная %s] %s", l.Number, msg)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		if _, err := fmt.Fprintf(l.file, "[%s] %s\n", time.Now().Format(timeLayout), msg); err != nil {
			log.Printf("NoteLogger %s: write note.txt: %v", l.Number, err)
		} else {
			// Попытка зафлашить данные на диск; если не поддерживается, логируем, но не прерываем работу
			if err := l.file.Sync(); err != nil {
				log.Printf("NoteLogger %s: sync note.txt: %v", l.Number, err)
			}
		}
	}
}

// SetStatus перезаписывает note.json с актуальным статусом подписания.
func (l *NoteLogger) SetStatus(status, errMsg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writeNoteJSON(l.note, status, errMsg)
}

func (l *NoteLogger) writeNoteJSON(n DeliveryNote, status, errMsg string) {
	path := filepath.Join(l.Dir, "note.json")
	now := time.Now().Format(timeLayout)
	processedAt := now
	if data, err := os.ReadFile(path); err == nil {
		var existing NoteFileJSON
		if json.Unmarshal(data, &existing) == nil && existing.ProcessedAt != "" {
			processedAt = existing.ProcessedAt
		}
	}
	f := NoteFileJSON{
		DeliveryNote: n,
		Status:       status,
		Error:        errMsg,
		ProcessedAt:  processedAt,
		UpdatedAt:    now,
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		log.Printf("NoteLogger %s: marshal note.json: %v", l.Number, err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("NoteLogger %s: write note.json: %v", l.Number, err)
	}
}

// saveScreenshot сохраняет скриншот в папку накладной с порядковым номером,
// чтобы на диске и на странице они шли по порядку (логин → подпись).
func (l *NoteLogger) saveScreenshot(name string, data []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.order++
	short := strings.TrimPrefix(name, l.Number+"_")
	path := filepath.Join(l.Dir, fmt.Sprintf("%02d_%s.png", l.order, short))
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("NoteLogger %s: save screenshot %s: %v", l.Number, path, err)
	}
}

// Screenshot снимает скриншот в контексте накладной через std chromedp flow.
// Сам вызов chromedp.FullScreenshot происходит в takeScreenshot; здесь только
// сохраняем уже снятый кадр, отснятый тем же активным потоком.
func (l *NoteLogger) Screenshot(ctx context.Context, name string) {
	takeScreenshot(ctx, l.Number+"_"+name)
}

func (l *NoteLogger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		if err := l.file.Close(); err != nil {
			log.Printf("NoteLogger %s: close note.txt: %v", l.Number, err)
		}
		l.file = nil
	}
}

func StartScreenshotsCleanup() {
	go func() {
		cleanupScreenshots()
		ticker := time.NewTicker(4 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			cleanupScreenshots()
		}
	}()
}

func cleanupScreenshots() {
	entries, err := os.ReadDir(screenshotDir)
	if err != nil {
		log.Printf("cleanupScreenshots: read dir: %v", err)
		return
	}
	type dirInfo struct {
		path string
		time time.Time
	}
	var dirs []dirInfo
	activeDir := ""
	if l := getActiveLogger(); l != nil {
		activeDir = l.Dir
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(screenshotDir, e.Name())
		if p == activeDir {
			continue
		}
		created := time.Time{}
		notePath := filepath.Join(p, "note.json")
		if data, err := os.ReadFile(notePath); err == nil {
			var nf NoteFileJSON
			if json.Unmarshal(data, &nf) == nil && nf.CreatedAt != "" {
				if t, err := time.Parse(timeLayout, nf.CreatedAt); err == nil {
					created = t
				}
			}
		}
		if created.IsZero() {
			if fi, err := os.Stat(p); err == nil {
				created = fi.ModTime()
			}
		}
		if created.IsZero() {
			created = time.Now()
		}
		dirs = append(dirs, dirInfo{path: p, time: created})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].time.After(dirs[j].time)
	})
	keep := 15
	if len(dirs) <= keep {
		return
	}
	for _, d := range dirs[keep:] {
		if d.path == activeDir {
			continue
		}
		log.Printf("cleanupScreenshots: removing %s", d.path)
		if err := os.RemoveAll(d.path); err != nil {
			log.Printf("cleanupScreenshots: remove %s: %v", d.path, err)
		}
	}
}
