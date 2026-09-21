package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type PushSender struct {
	cfg   Config
	store *Store
}

func newPushSender(cfg Config, store *Store) *PushSender {
	return &PushSender{cfg: cfg, store: store}
}

func (p *PushSender) enabled() bool {
	return p.cfg.VAPIDPublicKey != "" && p.cfg.VAPIDPrivateKey != ""
}

func (p *PushSender) deliver(ctx context.Context, reminder ReminderDelivery) (bool, string) {
	if !p.enabled() {
		return false, "Web Push is not configured"
	}
	subscriptions, err := p.store.listPushSubscriptions()
	if err != nil {
		return false, err.Error()
	}
	if len(subscriptions) == 0 {
		return false, "no active push subscriptions"
	}
	payload, _ := json.Marshal(map[string]any{
		"title":  "一个闪念 · 事项提醒",
		"body":   reminder.Title,
		"url":    "/agenda/?item=" + reminder.ItemID,
		"itemId": reminder.ItemID,
		"actions": []map[string]string{
			{"action": "complete", "title": "完成"},
			{"action": "snooze", "title": "推迟 10 分钟"},
		},
	})
	succeeded := 0
	lastError := ""
	for _, value := range subscriptions {
		subscription := &webpush.Subscription{Endpoint: value.Endpoint, Keys: webpush.Keys{P256dh: value.P256DH, Auth: value.Auth}}
		response, err := webpush.SendNotificationWithContext(ctx, payload, subscription, &webpush.Options{
			Subscriber:      p.cfg.VAPIDSubject,
			VAPIDPublicKey:  p.cfg.VAPIDPublicKey,
			VAPIDPrivateKey: p.cfg.VAPIDPrivateKey,
			TTL:             300,
		})
		if err != nil {
			lastError = err.Error()
			continue
		}
		response.Body.Close()
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			succeeded++
			continue
		}
		lastError = fmt.Sprintf("push endpoint returned HTTP %d", response.StatusCode)
		if response.StatusCode == http.StatusGone || response.StatusCode == http.StatusNotFound {
			_ = p.store.deactivatePushSubscription(value.Endpoint)
		}
	}
	if succeeded > 0 {
		return true, ""
	}
	return false, lastError
}

func (p *PushSender) run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reminders, err := p.store.dueReminders(25)
			if err != nil {
				log.Printf("reminder query failed: %v", err)
				continue
			}
			for _, reminder := range reminders {
				delivered, message := p.deliver(ctx, reminder)
				if message == "no active push subscriptions" {
					continue
				}
				if err := p.store.markReminder(reminder.ReminderID, delivered, message); err != nil {
					log.Printf("update reminder %s: %v", reminder.ReminderID, err)
				}
			}
		}
	}
}

func runBackupLoop(ctx context.Context, store *Store, dir string) {
	backup := func() {
		path, err := store.backup(dir)
		if err != nil {
			log.Printf("database backup failed: %v", err)
			return
		}
		log.Printf("database backup created: %s", path)
	}
	backup()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			backup()
		}
	}
}
