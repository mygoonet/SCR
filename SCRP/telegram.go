package SCRP

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

type TelegramClient struct {
	token  string
	chat   string
	client *http.Client
	mu     sync.Mutex
}

func NewTelegramClient(token, chat, socks5Addr string) *TelegramClient {
	var client *http.Client
	if socks5Addr != "" {
		dialer, err := proxy.SOCKS5("tcp", socks5Addr, nil, proxy.Direct)
		if err != nil {
			log.Printf("TelegramClient: ошибка создания SOCKS5 dialer: %v", err)
		} else {
			client = &http.Client{
				Transport: &http.Transport{
					DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						conn, err := dialer.Dial(network, addr)
						if err != nil {
							return nil, err
						}
						if dl, ok := conn.(*net.TCPConn); ok {
							dl.SetDeadline(time.Now().Add(10 * time.Second))
						}
						return conn, nil
					},
				},
			}
		}
	}
	if client == nil {
		client = &http.Client{}
	}

	return &TelegramClient{
		token:  token,
		chat:   chat,
		client: client,
	}
}

func (t *TelegramClient) Send(text string) error {
	if t.token == "" || t.chat == "" {
		return nil
	}

	data := url.Values{
		"chat_id": {t.chat},
		"text":    {text},
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	resp, err := t.client.PostForm(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token),
		data,
	)
	if err != nil {
		return fmt.Errorf("telegram POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func (t *TelegramClient) Sendf(format string, args ...interface{}) {
	if err := t.Send(fmt.Sprintf(format, args...)); err != nil {
		log.Printf("Telegram: %v", err)
	}
}
