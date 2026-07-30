package SCRP

import (
	"context"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

type Session struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
}

func (s *Session) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Session) Ctx() context.Context {
	return s.ctx
}

func DismissCookieBanner(ctx context.Context) {
	if ElementExists(ctx, "Понятно") {
		ClickElement(ctx, "Понятно")
		chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
	}
}
