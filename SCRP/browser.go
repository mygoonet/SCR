package SCRP

import (
	"context"
	"os"
	"path/filepath"

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
	}
}

type Browser struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewBrowser(cfg Config) *Browser {
	cleanStaleLock(cfg.UserDataDir)

	opts := buildChromeOpts(cfg)
	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	return &Browser{ctx: ctx, cancel: cancel}
}

func (b *Browser) NewSession() *Session {
	ctx, cancel := chromedp.NewContext(b.ctx)
	return &Session{ctx: ctx, cancel: cancel}
}

func (b *Browser) Close() {
	b.cancel()
}
