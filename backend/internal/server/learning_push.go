package server

import (
	"context"
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type learningPushKeys struct {
	public, private string
}

func (a *app) initLearningPush() error {
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return err
	}
	if _, err = a.db.Exec(`INSERT IGNORE INTO learning_push_keys(id,public_key,private_key) VALUES(1,?,?)`, public, private); err != nil {
		return err
	}
	return a.db.QueryRow(`SELECT public_key,private_key FROM learning_push_keys WHERE id=1`).Scan(&a.learningPush.public, &a.learningPush.private)
}

func validLearningPushSubscription(s webpush.Subscription) bool {
	u, err := url.Parse(s.Endpoint)
	if err != nil || len(s.Endpoint) > 2048 || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") || u.Fragment != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	allowed := host == "fcm.googleapis.com" || host == "updates.push.services.mozilla.com" || strings.HasSuffix(host, ".push.apple.com") || strings.HasSuffix(host, ".notify.windows.com")
	if !allowed {
		return false
	}
	key, err := base64.RawURLEncoding.DecodeString(s.Keys.P256dh)
	if err != nil {
		return false
	}
	if _, err = ecdh.P256().NewPublicKey(key); err != nil {
		return false
	}
	auth, err := base64.RawURLEncoding.DecodeString(s.Keys.Auth)
	return err == nil && len(auth) == 16
}

func (a *app) handleLearningPushKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"public_key": a.learningPush.public})
}

func (a *app) handleLearningPushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	hash := sha256.Sum256([]byte(req.Endpoint))
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM learning_push_subscriptions WHERE endpoint_hash=? AND user_id=?`, hash[:], mustUser(r).ID); err != nil {
		writeError(w, 500, "push_subscription_failed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *app) handleLearningPushSubscribe(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	member, err := a.learningReminderMember(r, groupID, u.ID)
	if err != nil {
		writeError(w, 500, "push_subscription_failed")
		return
	}
	if !member {
		writeError(w, 403, "group_membership_required")
		return
	}
	var subscription webpush.Subscription
	if !readJSON(w, r, &subscription) {
		return
	}
	if !validLearningPushSubscription(subscription) {
		writeError(w, 400, "invalid_push_subscription")
		return
	}
	data, err := json.Marshal(subscription)
	if err != nil {
		writeError(w, 400, "invalid_push_subscription")
		return
	}
	hash := sha256.Sum256([]byte(subscription.Endpoint))
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO learning_push_subscriptions(endpoint_hash,group_id,user_id,subscription,created_at) VALUES(?,?,?,?,UTC_TIMESTAMP(3)) ON DUPLICATE KEY UPDATE created_at=IF(group_id=VALUES(group_id) AND user_id=VALUES(user_id),created_at,VALUES(created_at)),group_id=VALUES(group_id),user_id=VALUES(user_id),subscription=VALUES(subscription)`, hash[:], groupID, u.ID, data)
	if err != nil {
		writeError(w, 500, "push_subscription_failed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *app) runLearningPush(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := a.enqueueDailyDevotionReminders(ctx, now); err != nil && ctx.Err() == nil {
				log.Print("daily devotion reminder scheduling failed")
			}
			if err := a.deliverLearningPush(ctx, client); err != nil && ctx.Err() == nil {
				log.Print("learning push queue processing failed")
			}
		}
	}
}

func (a *app) deliverLearningPush(ctx context.Context, client webpush.HTTPClient) error {
	// The reminder itself is the durable outbox. Only subscriptions already present
	// when it was sent are eligible, so enrolling a device cannot replay old reminders.
	_, err := a.db.ExecContext(ctx, `INSERT IGNORE INTO learning_push_deliveries(reminder_id,subscription_id,next_attempt_at) SELECT r.id,s.id,UTC_TIMESTAMP(3) FROM learning_reminders r JOIN learning_push_subscriptions s ON s.group_id=r.group_id AND s.user_id=r.recipient_id AND s.created_at<=r.created_at WHERE r.created_at>UTC_TIMESTAMP()-INTERVAL 1 DAY AND r.read_at IS NULL`)
	if err != nil {
		return err
	}
	rows, err := a.db.QueryContext(ctx, `SELECT d.reminder_id,d.subscription_id,d.attempts,s.subscription,CASE WHEN r.sender_id=r.recipient_id THEN '每日灵修' ELSE COALESCE(NULLIF(gm.member_name,''),u.display_name) END,r.group_id FROM learning_push_deliveries d JOIN learning_reminders r ON r.id=d.reminder_id JOIN learning_push_subscriptions s ON s.id=d.subscription_id AND s.user_id=r.recipient_id AND s.group_id=r.group_id JOIN group_members recipient ON recipient.group_id=r.group_id AND recipient.user_id=r.recipient_id AND recipient.status=1 JOIN users u ON u.id=r.sender_id LEFT JOIN group_members gm ON gm.group_id=r.group_id AND gm.user_id=r.sender_id LEFT JOIN learning_reminder_preferences p ON p.group_id=r.group_id AND p.user_id=r.recipient_id LEFT JOIN group_settings gs ON gs.group_id=r.group_id WHERE d.finished=FALSE AND d.attempts<3 AND d.next_attempt_at<=UTC_TIMESTAMP(3) AND r.read_at IS NULL AND r.created_at>UTC_TIMESTAMP()-INTERVAL 1 DAY AND COALESCE(p.muted,FALSE)=FALSE AND (r.sender_id<>r.recipient_id OR (COALESCE(JSON_UNQUOTE(JSON_EXTRACT(gs.settings,'$.checkin_notifications.daily_enabled')),'true')='true' AND r.logical_date=DATE(UTC_TIMESTAMP()+INTERVAL 8 HOUR))) AND (r.id>COALESCE(p.apple_push_after_id,0) OR JSON_UNQUOTE(JSON_EXTRACT(s.subscription,'$.endpoint')) NOT LIKE 'https://%.push.apple.com/%') ORDER BY d.reminder_id DESC LIMIT 20`)
	if err != nil {
		return err
	}
	type delivery struct {
		id, subscriptionID, groupID uint64
		attempts                    int
		data                        []byte
		sender                      string
	}
	var items []delivery
	for rows.Next() {
		var item delivery
		if err = rows.Scan(&item.id, &item.subscriptionID, &item.attempts, &item.data, &item.sender, &item.groupID); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, item)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	verses := []string{"你们要靠主常常喜乐。", "我们行善，不可丧志。", "你当刚强壮胆！", "你的话是我脚前的灯，是我路上的光。", "在指望中要喜乐，在患难中要忍耐。"}
	for _, item := range items {
		claimed, claimErr := a.db.ExecContext(ctx, `UPDATE learning_push_deliveries SET attempts=attempts+1,next_attempt_at=UTC_TIMESTAMP(3)+INTERVAL 1 MINUTE WHERE reminder_id=? AND subscription_id=? AND attempts=? AND finished=FALSE AND next_attempt_at<=UTC_TIMESTAMP(3)`, item.id, item.subscriptionID, item.attempts)
		if claimErr != nil {
			return claimErr
		}
		count, claimErr := claimed.RowsAffected()
		if claimErr != nil {
			return claimErr
		}
		if count != 1 {
			continue
		}
		var subscription webpush.Subscription
		if json.Unmarshal(item.data, &subscription) != nil || !validLearningPushSubscription(subscription) {
			continue
		}
		payload, marshalErr := json.Marshal(map[string]any{"id": item.id, "group_id": item.groupID, "title": item.sender + " 提醒你打卡", "body": verses[item.id%uint64(len(verses))]})
		if marshalErr != nil {
			return marshalErr
		}
		response, sendErr := webpush.SendNotificationWithContext(ctx, payload, &subscription, &webpush.Options{HTTPClient: client, Subscriber: "https://github.com/wangz5940/cedar-discipleship", VAPIDPublicKey: a.learningPush.public, VAPIDPrivateKey: a.learningPush.private, TTL: 3600, Topic: fmt.Sprintf("reminder-%d", item.id), Urgency: webpush.UrgencyHigh})
		if sendErr != nil {
			log.Printf("learning push send failed reminder=%d attempt=%d", item.id, item.attempts+1)
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode == 404 || response.StatusCode == 410 {
			_, err = a.db.ExecContext(ctx, `DELETE FROM learning_push_subscriptions WHERE id=? AND group_id=? AND user_id=(SELECT recipient_id FROM learning_reminders WHERE id=?) AND subscription=CAST(? AS JSON)`, item.subscriptionID, item.groupID, item.id, item.data)
		} else if response.StatusCode >= 200 && response.StatusCode < 300 {
			_, err = a.db.ExecContext(ctx, `UPDATE learning_push_deliveries SET finished=TRUE WHERE reminder_id=? AND subscription_id=?`, item.id, item.subscriptionID)
			log.Printf("learning push accepted reminder=%d", item.id)
		} else {
			log.Printf("learning push rejected reminder=%d status=%d attempt=%d", item.id, response.StatusCode, item.attempts+1)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
