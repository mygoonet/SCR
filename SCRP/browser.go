package SCRP

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

func cleanStaleLock(dir string) {
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		os.Remove(filepath.Join(dir, name))
	}
}

func buildChromeOpts(cfg Config) []chromedp.ExecAllocatorOption {

	return []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(cfg.ChromePath),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Headless,
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-popup-blocking", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("enable-automation", true),
		chromedp.Flag("password-store", "basic"),
		chromedp.Flag("use-mock-keychain", true),
		chromedp.Flag("disable-features", "Translate"),
		chromedp.Flag("window-size", "1920,1080"),
		chromedp.Flag("user-data-dir", cfg.UserDataDir),
		// Кэш отключён навсегда — свежий DOM на каждой перезагрузке,
		// меньше гонок вида "элемент есть, но невидим".
		// chromedp принимает только string или bool (int → "invalid exec pool flag").
		chromedp.Flag("disk-cache-size", "0"),
		chromedp.Flag("disable-application-cache", true),
	}
}

type Browser struct {
	cfg Config

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

// CheckChromeBinary — предпроверка бинарника chromium: существует,
// исполняемый, запускается и отвечает на --version.
//
// Зачем: если запущенный процесс хрома завершается немедленно (битый путь,
// упавшая библиотека, OOM), chromedp v0.16.0 паникует «close of closed
// channel» в горутине своего аллокатора (allocate.go:233). recover() в
// MonitorAPI такую панику не перехватывает (другая горутина) — процесс
// умирает целиком, и подписание возвращалось только внешним restart
// контейнера. Предпроверка не даёт дойти до Allocate с нерабочим бинарником.
func CheckChromeBinary(path string) error {
	if path == "" {
		return fmt.Errorf("CHROME_PATH пустой")
	}
	fi, err := os.Stat(path)
	switch {
	case err != nil:
		return fmt.Errorf("chromium по CHROME_PATH=%s не найден: %w", path, err)
	case fi.IsDir():
		return fmt.Errorf("CHROME_PATH=%s — каталог, а не бинарь", path)
	case fi.Mode().Perm()&0111 == 0:
		return fmt.Errorf("CHROME_PATH=%s не исполняемый (права %v)", path, fi.Mode().Perm())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return fmt.Errorf("chromium %s не отвечает на --version: %w", path, err)
	}
	log.Printf("Browser: chromium готов: %s", strings.TrimSpace(string(out)))
	return nil
}

func NewBrowser(cfg Config) *Browser {
	b := &Browser{cfg: cfg}
	// Пред-подъём хрома. Ошибка уже залогирована в allocate(); NewSession
	// повторит попытку позже — мониторинг переждёт её по backoff.
	b.allocate()
	return b
}

// allocate поднимает chromium, если текущего аллокатора нет или он мёртв
// (контекст отменён после смерти процесса). Возвращает true, если есть
// живой контекст для новой сессии.
func (b *Browser) allocate() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.ctx != nil && b.ctx.Err() == nil {
		return true // живой аллокатор переиспользуем
	}
	if b.cancel != nil {
		b.cancel() // добиваем старый (уже мёртвый) контекст
		b.ctx, b.cancel = nil, nil
	}

	if err := CheckChromeBinary(b.cfg.ChromePath); err != nil {
		log.Printf("Browser: предпроверка chromium не прошла: %v", err)
		return false
	}

	// После смерти/убийства процесса хрома в профиле остаётся SingletonLock —
	// новая инстанция на нём не поднимется.
	cleanStaleLock(b.cfg.UserDataDir)

	filteredLogger := func(format string, v ...any) {
		if strings.Contains(format, "unhandled node event") {
			return // Игнорируем unhandled node event
		}
		log.Printf(format, v...)
	}

	opts := buildChromeOpts(b.cfg)
	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(filteredLogger))

	// Форсируем аллокацию браузера на этом контексте, чтобы browserOpts
	// (включая WithErrorf) были переданы в Allocate. Без этого первый Run
	// на дочернем контексте (NewSession) аллоцирует браузер без опций.
	if err := chromedp.Run(ctx, chromedp.Navigate("about:blank")); err != nil {
		cancel()
		log.Printf("Browser: chromium не поднялся: %v", err)
		return false
	}

	b.ctx, b.cancel = ctx, cancel
	log.Println("Browser: chromium поднят")
	return true
}

// NewSession возвращает сессию поверх живого аллокатора. Если chromium умер
// (или не поднимается), вместо сессии на мёртвом контексте отдаётся сессия
// с уже отменённым контекстом: любой Run на ней мгновенно вернёт ошибку,
// и MonitorAPI повторит попытку после backoff, по пути переподнимая хром.
func (b *Browser) NewSession() *Session {
	if !b.allocate() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return &Session{ctx: ctx, cancel: cancel}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	ctx, cancel := chromedp.NewContext(b.ctx)
	return &Session{ctx: ctx, cancel: cancel}
}

func (b *Browser) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancel != nil {
		b.cancel()
		b.ctx, b.cancel = nil, nil
	}
}
