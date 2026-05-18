// Package notify dispatches panel events to webhook and Telegram sinks.
// It runs as a single goroutine, subscribed to the events broker, and
// re-reads stack config on each event so live changes take effect without
// restart.
package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"

	"github.com/sakhtar/xray-stack-zeroone/internal/events"
	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

type Notifier struct {
	Broker     *events.Broker
	ConfigRead func() stack.Config
	HTTPClient *http.Client

	// proxiedClient is built lazily — Telegram on Iranian-hosted servers is
	// firewall-blocked over the public internet, so we tunnel api.telegram.org
	// through Xray's local SOCKS inbound (which uses the proxy outbound).
	muClients     sync.Mutex
	proxiedClient *http.Client
	proxiedPort   int
}

// telegramClient returns an HTTP client whose dialer routes through the
// daemon's local SOCKS inbound. Falls back to the default HTTPClient if the
// local SOCKS port isn't configured.
func (n *Notifier) telegramClient(port int) *http.Client {
	if port <= 0 {
		return n.HTTPClient
	}
	n.muClients.Lock()
	defer n.muClients.Unlock()
	if n.proxiedClient != nil && n.proxiedPort == port {
		return n.proxiedClient
	}
	d, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", port), nil, proxy.Direct)
	if err != nil {
		slog.Warn("notify: socks5 dialer", "err", err)
		return n.HTTPClient
	}
	cd, ok := d.(proxy.ContextDialer)
	if !ok {
		slog.Warn("notify: socks5 ContextDialer not supported")
		return n.HTTPClient
	}
	tr := &http.Transport{
		DialContext:           cd.DialContext,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
	}
	n.proxiedClient = &http.Client{Transport: tr, Timeout: 15 * time.Second}
	n.proxiedPort = port
	return n.proxiedClient
}

// suppress unused imports when refactoring later.
var _ = net.IPv4

func (n *Notifier) Run(ctx context.Context) {
	if n.HTTPClient == nil {
		n.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	ch := n.Broker.Subscribe()
	defer n.Broker.Unsubscribe(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			cfg := n.ConfigRead()
			socksPort := cfg.Xray.Inbounds.LocalSOCKSPort
			if socksPort == 0 {
				socksPort = 10808
			}
			if shouldDeliver(cfg.Panel.Notifications.Webhook.Events, ev.Kind) && cfg.Panel.Notifications.Webhook.URL != "" {
				go n.sendWebhook(ctx, cfg.Panel.Notifications.Webhook, ev)
			}
			if shouldDeliver(cfg.Panel.Notifications.Telegram.Events, ev.Kind) && cfg.Panel.Notifications.Telegram.BotToken != "" && cfg.Panel.Notifications.Telegram.ChatID != "" {
				go n.sendTelegram(ctx, cfg.Panel.Notifications.Telegram, socksPort, ev)
			}
		}
	}
}

func shouldDeliver(want []string, kind string) bool {
	if len(want) == 0 {
		return false
	}
	for _, w := range want {
		if w == "*" || w == kind {
			return true
		}
	}
	return false
}

func (n *Notifier) sendWebhook(ctx context.Context, sink stack.WebhookSink, ev events.Event) {
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sink.URL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "xray-stackd-notifier/1")
	if sink.Secret != "" {
		mac := hmac.New(sha256.New, []byte(sink.Secret))
		mac.Write(body)
		req.Header.Set("X-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := n.HTTPClient.Do(req)
	if err != nil {
		slog.Warn("webhook send failed", "err", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		slog.Warn("webhook non-2xx", "status", resp.StatusCode)
	}
}

func (n *Notifier) sendTelegram(ctx context.Context, sink stack.TelegramSink, socksPort int, ev events.Event) {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", sink.BotToken)
	form := url.Values{}
	form.Set("chat_id", sink.ChatID)
	form.Set("text", formatTelegramMessage(ev))
	form.Set("parse_mode", "Markdown")
	form.Set("disable_web_page_preview", "true")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := n.telegramClient(socksPort).Do(req)
	if err != nil {
		slog.Warn("telegram send failed", "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		slog.Warn("telegram non-2xx", "status", resp.StatusCode, "body", string(body))
	}
}

// TelegramChat is one entry returned by Discover. Useful for filling chat_id.
type TelegramChat struct {
	ChatID int64  `json:"chat_id"`
	Title  string `json:"title"`
	Type   string `json:"type"`
	From   string `json:"from,omitempty"`
}

// DiscoverTelegramChats hits getUpdates via the local SOCKS so the panel can
// surface a list of recent chats — the operator picks one to fill chat_id.
// Requires the user to have sent at least one message to the bot first.
func (n *Notifier) DiscoverTelegramChats(ctx context.Context, botToken string, socksPort int) ([]TelegramChat, error) {
	if botToken == "" {
		return nil, fmt.Errorf("bot_token is empty")
	}
	if n.HTTPClient == nil {
		n.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates", botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := n.telegramClient(socksPort).Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram unreachable (firewall? local SOCKS down?): %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("telegram %d: %s", resp.StatusCode, string(body))
	}
	var payload struct {
		OK     bool `json:"ok"`
		Result []struct {
			Message *struct {
				From *struct {
					Username  string `json:"username"`
					FirstName string `json:"first_name"`
				} `json:"from"`
				Chat *struct {
					ID    int64  `json:"id"`
					Title string `json:"title"`
					First string `json:"first_name"`
					Type  string `json:"type"`
				} `json:"chat"`
			} `json:"message"`
			MyChatMember *struct {
				Chat *struct {
					ID    int64  `json:"id"`
					Title string `json:"title"`
					First string `json:"first_name"`
					Type  string `json:"type"`
				} `json:"chat"`
			} `json:"my_chat_member"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode getUpdates: %w", err)
	}
	if !payload.OK {
		return nil, fmt.Errorf("telegram api returned ok=false: %s", string(body))
	}
	seen := map[int64]bool{}
	out := []TelegramChat{}
	add := func(id int64, title, first, kind, from string) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		t := title
		if t == "" {
			t = first
		}
		out = append(out, TelegramChat{ChatID: id, Title: t, Type: kind, From: from})
	}
	for _, u := range payload.Result {
		if u.Message != nil && u.Message.Chat != nil {
			from := ""
			if u.Message.From != nil {
				if u.Message.From.Username != "" {
					from = "@" + u.Message.From.Username
				} else {
					from = u.Message.From.FirstName
				}
			}
			add(u.Message.Chat.ID, u.Message.Chat.Title, u.Message.Chat.First, u.Message.Chat.Type, from)
		}
		if u.MyChatMember != nil && u.MyChatMember.Chat != nil {
			add(u.MyChatMember.Chat.ID, u.MyChatMember.Chat.Title, u.MyChatMember.Chat.First, u.MyChatMember.Chat.Type, "")
		}
	}
	return out, nil
}

// tehranTZ pins notification timestamps to Iran time regardless of the
// host's locale, matching the panel UI which always renders in Asia/Tehran.
var tehranTZ = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Tehran"); err == nil {
		return loc
	}
	return time.FixedZone("IRST", 3*3600+30*60)
}()

func formatTelegramMessage(ev events.Event) string {
	t := time.Unix(ev.Time, 0).In(tehranTZ).Format("2006-01-02 15:04:05")
	var sb strings.Builder
	fmt.Fprintf(&sb, "*Xray Stack* — `%s`\n", ev.Kind)
	fmt.Fprintf(&sb, "_%s_\n", t)
	for k, v := range ev.Data {
		fmt.Fprintf(&sb, "• %s: `%v`\n", k, v)
	}
	return sb.String()
}
