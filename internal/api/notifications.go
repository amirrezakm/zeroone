package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/notify"
	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

func (s *Server) notificationsGet(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Panel.Notifications
	// Redact webhook secret + telegram bot token before returning
	view := map[string]any{
		"webhook": map[string]any{
			"url":            cfg.Webhook.URL,
			"events":         cfg.Webhook.Events,
			"secret_set":     cfg.Webhook.Secret != "",
		},
		"telegram": map[string]any{
			"chat_id":        cfg.Telegram.ChatID,
			"events":         cfg.Telegram.Events,
			"bot_token_set":  cfg.Telegram.BotToken != "",
		},
	}
	s.write(w, map[string]any{"ok": true, "notifications": view})
}

func (s *Server) notificationsPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Webhook  *stack.WebhookSink  `json:"webhook,omitempty"`
		Telegram *stack.TelegramSink `json:"telegram,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if req.Webhook != nil {
		// Empty secret ⇒ keep existing (allow update without re-typing).
		if req.Webhook.Secret == "" {
			req.Webhook.Secret = s.cfg.Panel.Notifications.Webhook.Secret
		}
		s.cfg.Panel.Notifications.Webhook = *req.Webhook
	}
	if req.Telegram != nil {
		if req.Telegram.BotToken == "" {
			req.Telegram.BotToken = s.cfg.Panel.Notifications.Telegram.BotToken
		}
		s.cfg.Panel.Notifications.Telegram = *req.Telegram
	}
	if !s.save(w) {
		return
	}
	s.recordAudit(s.actor(r), "notifications.update", "", nil)
	s.write(w, map[string]any{"ok": true})
}

func (s *Server) notificationsTest(w http.ResponseWriter, r *http.Request) {
	if s.events == nil {
		s.fail(w, http.StatusServiceUnavailable, nil)
		return
	}
	s.events.Publish("test", map[string]any{"message": "panel test notification", "actor": s.actor(r)})
	s.recordAudit(s.actor(r), "notifications.test", "", nil)
	s.write(w, map[string]any{"ok": true, "note": "test event published — webhook/telegram should fire if configured for kind=test or kind=*"})
}

// telegramChats helps an operator find the chat_id to use. It calls
// getUpdates via the daemon's local SOCKS so it works on Iranian-hosted
// servers where api.telegram.org is blocked over the public path.
// Requires the user to have first started a chat with the bot.
func (s *Server) telegramChats(w http.ResponseWriter, r *http.Request) {
	token := s.cfg.Panel.Notifications.Telegram.BotToken
	if token == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("telegram bot_token not configured"))
		return
	}
	port := s.cfg.Xray.Inbounds.LocalSOCKSPort
	if port == 0 {
		port = 10808
	}
	n := &notify.Notifier{}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	chats, err := n.DiscoverTelegramChats(ctx, token, port)
	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}
	hint := ""
	if len(chats) == 0 {
		hint = "no recent chats — open Telegram, search for the bot, send /start, then retry"
	}
	s.write(w, map[string]any{"ok": true, "chats": chats, "hint": hint})
}
