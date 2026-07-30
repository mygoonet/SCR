package SCRP

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Scraper — обёртка над Session для периодического опроса и подписания накладных.
type Scraper struct {
	session   *Session
	signCh    chan string
	signAllCh chan chan error
	listCh    chan chan []DeliveryNote
}

// NewScraper создаёт новый скрепер с заданной конфигурацией.
func NewScraper(cfg Config) *Scraper {
	return &Scraper{
		session:   New(cfg),
		signCh:    make(chan string, 10),
		signAllCh: make(chan chan error, 1),
		listCh:    make(chan chan []DeliveryNote, 1),
	}
}

// Start запускает цикл: каждые 50 секунд выводит список и подписывает все накладные.
func (sc *Scraper) Start() error {
	if err := sc.session.Open(); err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	defer sc.session.Close()

	ctx := sc.session.Ctx()

	if err := sc.initSession(ctx); err != nil {
		return fmt.Errorf("init session: %w", err)
	}

	log.Println("Scraper запущен. Тикер 50с: вывод + автоподпись всех")

	ticker := time.NewTicker(50 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case number := <-sc.signCh:
			log.Printf(">>> Подписание накладной %s...", number)
			go sc.signOne(ctx, number)

		case listResp := <-sc.listCh:
			notes, err := sc.fetchNotes(ctx)
			if err != nil {
				log.Printf("Ошибка получения накладных: %v", err)
				listResp <- nil
			} else {
				listResp <- notes
			}

		case respCh := <-sc.signAllCh:
			go func() {
				respCh <- sc.signAllInCtx(ctx)
			}()

		case <-ticker.C:
			notes, err := sc.fetchNotes(ctx)
			if err != nil {
				log.Printf("Ошибка получения накладных: %v", err)
			} else {
				log.Printf("Накладные (%d):", len(notes))
				for _, n := range notes {
					log.Printf("  %s  от %s  %s → %s  %s", n.Number, n.Date, n.Consignor, n.Consignee, n.Carrier)
				}

				log.Printf(">>> SignAll: подписываю %d накладных", len(notes))
				go func() {
					if err := sc.signAllInCtx(ctx); err != nil {
						log.Printf(">>> SignAll ошибка: %v", err)
					} else {
						log.Println(">>> SignAll завершено")
					}
				}()
			}
		}
	}
}

func (sc *Scraper) signOne(ctx context.Context, number string) {
	if err := SignDeliveryNote(ctx, number, sc.session.Cfg().CertUser); err != nil {
		log.Printf(">>> Ошибка подписания %s: %v", number, err)
	} else {
		log.Printf(">>> Накладная %s подписана", number)
	}
}

func (sc *Scraper) signAllInCtx(ctx context.Context) error {
	notes, err := sc.fetchNotes(ctx)
	if err != nil {
		return fmt.Errorf("получить список: %w", err)
	}

	var firstErr error
	for _, n := range notes {
		log.Printf(">>> SignAll: подписываю %s...", n.Number)
		if err := SignDeliveryNote(ctx, n.Number, sc.session.Cfg().CertUser); err != nil {
			log.Printf(">>> Ошибка подписания %s: %v", n.Number, err)
			if firstErr == nil {
				firstErr = err
			}
		} else {
			log.Printf(">>> Накладная %s подписана", n.Number)
		}
	}
	return firstErr
}

// Sign отправляет запрос на подпись одной накладной.
func (sc *Scraper) Sign(number string) {
	select {
	case sc.signCh <- number:
	default:
		log.Printf(">>> signCh переполнен, пропускаю %s", number)
	}
}

// SignAll подписывает все текущие накладные.
func (sc *Scraper) SignAll() <-chan error {
	resp := make(chan error, 1)
	select {
	case sc.signAllCh <- resp:
	default:
		log.Printf(">>> signAllCh переполнен, пропускаю запрос")
	}
	return resp
}

// GetListNotes асинхронно получает список накладных.
func (sc *Scraper) GetListNotes() <-chan []DeliveryNote {
	resp := make(chan []DeliveryNote, 1)
	select {
	case sc.listCh <- resp:
	default:
		log.Printf(">>> listCh переполнен, пропускаю запрос")
	}
	return resp
}

func (sc *Scraper) initSession(ctx context.Context) error {
	if err := NavigateToLogin(ctx); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}
	dismissCookieBanner(ctx)

	if ElementExists(ctx, "Сертификат") {
		if err := Login(ctx, sc.session.cfg.CertUser); err != nil {
			return fmt.Errorf("login: %w", err)
		}
	}

	if err := NavigateToCarrier(ctx); err != nil {
		return fmt.Errorf("carrier: %w", err)
	}
	return nil
}

func (sc *Scraper) fetchNotes(ctx context.Context) ([]DeliveryNote, error) {
	notes, err := ParseDeliveryNotes(ctx)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	sc.session.mu.Lock()
	sc.session.notes = notes
	sc.session.mu.Unlock()
	return notes, nil
}
