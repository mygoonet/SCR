package SCRP

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

type Session struct {
	cfg    Config
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	state  State
	notes  []DeliveryNote
}

func New(cfg Config) *Session {
	return &Session{cfg: cfg, state: StateClosed}
}

func (s *Session) Open() error {
	if s.state != StateClosed {
		return fmt.Errorf("session already open (%s)", s.state)
	}
	s.ctx, s.cancel = NewBrowser(s.cfg)
	s.state = StateOpen
	return nil
}

func (s *Session) Ctx() context.Context {
	return s.ctx
}

func (s *Session) Run() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateClosed {
		return fmt.Errorf("session closed, call Open() first")
	}
	if s.state == StateError {
		return fmt.Errorf("session in error state, call Open() to reset")
	}

	if err := NavigateToLogin(s.ctx); err != nil {
		s.state = StateError
		return fmt.Errorf("navigate: %w", err)
	}

	dismissCookieBanner(s.ctx)

	if ElementExists(s.ctx, "Сертификат") {
		s.state = StateLoggedOut
		if err := Login(s.ctx, s.cfg.CertUser); err != nil {
			s.state = StateError
			return fmt.Errorf("login: %w", err)
		}
	}

	s.state = StateLoggedIn

	if err := NavigateToCarrier(s.ctx); err != nil {
		s.state = StateError
		return fmt.Errorf("carrier: %w", err)
	}

	notes, err := ParseDeliveryNotes(s.ctx)
	if err != nil {
		s.state = StateError
		return fmt.Errorf("parse: %w", err)
	}

	s.notes = notes
	return nil
}

func dismissCookieBanner(ctx context.Context) {
	if ElementExists(ctx, "Понятно") {
		ClickElement(ctx, "Понятно")
		chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
	}
}

func (s *Session) DeliveryNotes() []DeliveryNote {
	return s.notes
}

func (s *Session) State() State {
	return s.state
}

func (s *Session) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	s.state = StateClosed
}
