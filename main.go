package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"SCR/SCRP"
)

type Runner struct {
	session *SCRP.Session
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewRunner(session *SCRP.Session) *Runner {
	return &Runner{session: session}
}

func (r *Runner) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cancel != nil {
		log.Println("already running")
		return
	}

	r.ctx, r.cancel = context.WithCancel(context.Background())
	r.wg.Add(1)
	go r.loop()
}

func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cancel == nil {
		return
	}

	r.cancel()
	r.wg.Wait()
	r.cancel = nil
}

func (r *Runner) Sign(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cancel == nil {
		return fmt.Errorf("runner not running, call Start() first")
	}

	return SCRP.SignDeliveryNote(r.session.Ctx(), id)
}

func (r *Runner) loop() {
	defer r.wg.Done()

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		if err := r.session.Run(); err != nil {
			log.Println(err)
			select {
			case <-r.ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}

		notes := r.session.DeliveryNotes()
		b, _ := json.MarshalIndent(notes, "", "  ")
		log.Printf("Получено %d накладных:\n%s", len(notes), string(b))

		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func main() {
	s := SCRP.New(SCRP.Config{
		UserDataDir: "/home/visa/.config/google-chrome",
		ChromePath:  "/usr/bin/chromium-gost-stable",
		CertUser:    "Сичкарук Евгений Александрович",
	})

	if err := s.Open(); err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	runner := NewRunner(s)
	runner.Start()


	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("остановка...")
	runner.Stop()
}
