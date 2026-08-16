package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

const (
	channelGatewayVersion       = "1.7.0"
	channelDeliveryMaxAttempts  = 6
	channelDeliveryPollInterval = 2 * time.Second
	channelReceiptRetention     = 30 * 24 * time.Hour
)

type ChannelRoute struct {
	ID           string    `json:"id"`
	ChannelID    string    `json:"channel_id"`
	SessionID    string    `json:"session_id"`
	SenderDigest string    `json:"sender_sha256_96"`
	ReplyTarget  string    `json:"reply_target"`
	Thread       string    `json:"thread,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type OutboundDelivery struct {
	ID            string            `json:"id"`
	ChannelID     string            `json:"channel_id"`
	SessionID     string            `json:"session_id"`
	TaskID        string            `json:"task_id"`
	RouteID       string            `json:"route_id"`
	Status        string            `json:"status"`
	Attempts      int               `json:"attempts"`
	NextAttemptAt time.Time         `json:"next_attempt_at"`
	LastError     string            `json:"last_error,omitempty"`
	Secrets       map[string]string `json:"secrets,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type OutboundReceipt struct {
	ID              string     `json:"id"`
	ChannelID       string     `json:"channel_id"`
	SessionID       string     `json:"session_id"`
	TaskID          string     `json:"task_id"`
	RouteID         string     `json:"route_id"`
	RecipientDigest string     `json:"recipient_sha256_96"`
	Status          string     `json:"status"`
	Attempts        int        `json:"attempts"`
	LastError       string     `json:"last_error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

type channelGatewayV170 struct {
	manager *Manager
	once    sync.Once
}

var channelGatewaysV170 sync.Map

func (m *Manager) gatewayV170() *channelGatewayV170 {
	v, _ := channelGatewaysV170.LoadOrStore(m, &channelGatewayV170{manager: m})
	g := v.(*channelGatewayV170)
	g.once.Do(func() {
		for _, name := range []string{"channel-routes", "outbound-pending", "outbound-receipts"} {
			_ = os.MkdirAll(filepath.Join(m.dir, name), 0o700)
		}
		m.wg.Add(1)
		go g.loop()
	})
	return g
}

func (g *channelGatewayV170) loop() {
	defer g.manager.wg.Done()
	defer channelGatewaysV170.Delete(g.manager)
	deliveryTicker := time.NewTicker(channelDeliveryPollInterval)
	pruneTicker := time.NewTicker(time.Hour)
	defer deliveryTicker.Stop()
	defer pruneTicker.Stop()
	g.pruneReceipts()
	for {
		select {
		case <-g.manager.ctx.Done():
			return
		case <-deliveryTicker.C:
			g.processPending()
		case <-pruneTicker.C:
			g.pruneReceipts()
		}
	}
}

func channelRouteID(channelID, sender, target, thread string) string {
	h := sha256.Sum256([]byte(channelID + "\x00" + sender + "\x00" + target + "\x00" + thread))
	return "route_" + hex.EncodeToString(h[:16])
}

func outboundDeliveryID(channelID, taskID string) string {
	h := sha256.Sum256([]byte(channelID + "\x00" + taskID))
	return "out_" + hex.EncodeToString(h[:16])
}

func pseudonymousSender(raw, routeID string) string {
	suffix := routeID
	if len(suffix) > 14 {
		suffix = suffix[len(suffix)-14:]
	}
	return "external:" + senderDigest(raw) + ":" + suffix
}

func (g *channelGatewayV170) route(channel *Channel, rawSender, target, thread string) (*ChannelRoute, error) {
	m := g.manager
	id := channelRouteID(channel.ID, rawSender, target, thread)
	m.mu.Lock()
	defer m.mu.Unlock()
	var existing ChannelRoute
	if err := m.read("channel-routes", id, &existing); err == nil {
		existing.UpdatedAt = now()
		if err := m.save("channel-routes", id, &existing); err != nil {
			return nil, err
		}
		return &existing, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	sessionID, err := storage.RandomID("sess")
	if err != nil {
		return nil, err
	}
	t := now()
	session := Session{ID: sessionID, Channel: channel.ID, Sender: pseudonymousSender(rawSender, id), Turns: []SessionTurn{}, CreatedAt: t, UpdatedAt: t}
	route := &ChannelRoute{ID: id, ChannelID: channel.ID, SessionID: sessionID, SenderDigest: senderDigest(rawSender), ReplyTarget: target, Thread: thread, CreatedAt: t, UpdatedAt: t}
	if err := m.audit("channel.route.created", map[string]any{"channel_id": channel.ID, "route_id": id, "session_id": sessionID, "sender_sha256_96": route.SenderDigest}); err != nil {
		return nil, err
	}
	if err := m.save("sessions", sessionID, &session); err != nil {
		return nil, err
	}
	if err := m.save("channel-routes", id, route); err != nil {
		return nil, err
	}
	return route, nil
}

func (g *channelGatewayV170) ensurePending(channel *Channel, route *ChannelRoute, taskID string, secrets map[string]string) (*OutboundDelivery, error) {
	m := g.manager
	id := outboundDeliveryID(channel.ID, taskID)
	m.mu.Lock()
	defer m.mu.Unlock()
	var existing OutboundDelivery
	if err := m.read("outbound-pending", id, &existing); err == nil {
		return &existing, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	t := now()
	d := &OutboundDelivery{ID: id, ChannelID: channel.ID, SessionID: route.SessionID, TaskID: taskID, RouteID: route.ID, Status: "waiting_task", NextAttemptAt: t, Secrets: secrets, CreatedAt: t, UpdatedAt: t}
	if err := m.audit("channel.outbound.queued", map[string]any{"channel_id": channel.ID, "delivery_id": id, "task_id": taskID, "route_id": route.ID}); err != nil {
		return nil, err
	}
	if err := m.save("outbound-pending", id, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (g *channelGatewayV170) processPending() {
	m := g.manager
	m.mu.RLock()
	pending, err := listJSON[OutboundDelivery](filepath.Join(m.dir, "outbound-pending"))
	m.mu.RUnlock()
	if err != nil {
		return
	}
	for _, d := range pending {
		select {
		case <-m.ctx.Done():
			return
		default:
		}
		if d.Status == "reconciliation" || d.Status == "failed" || d.NextAttemptAt.After(now()) {
			continue
		}
		g.processDelivery(d.ID)
	}
}

func (g *channelGatewayV170) processDelivery(id string) {
	m := g.manager
	m.mu.RLock()
	var d OutboundDelivery
	if err := m.read("outbound-pending", id, &d); err != nil {
		m.mu.RUnlock()
		return
	}
	m.mu.RUnlock()
	if d.Status == "sending" {
		g.markReconciliation(&d, "delivery outcome unknown after restart")
		return
	}

	t, err := m.rt.Task(d.TaskID)
	if err != nil {
		return
	}
	switch t.Status {
	case task.Completed:
	case task.Failed, task.Canceled:
		g.complete(&d, "task_failed", memorySafeError(errors.New(t.Error)))
		return
	default:
		return
	}

	m.mu.RLock()
	var channel Channel
	channelErr := m.read("channels", d.ChannelID, &channel)
	var route ChannelRoute
	routeErr := m.read("channel-routes", d.RouteID, &route)
	m.mu.RUnlock()
	if channelErr != nil || routeErr != nil || !channel.Enabled {
		g.complete(&d, "failed", "channel or route unavailable")
		return
	}

	d.Status = "sending"
	d.Attempts++
	d.UpdatedAt = now()
	m.mu.Lock()
	if err := m.save("outbound-pending", d.ID, &d); err != nil {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	_, sendErr := g.send(m.ctx, &channel, &route, t.Output, d.Secrets)
	if sendErr == nil {
		g.complete(&d, "delivered", "")
		return
	}
	if isRetryableChannelError(sendErr) && d.Attempts < channelDeliveryMaxAttempts {
		d.Status = "retry_wait"
		d.LastError = safeChannelError(sendErr)
		d.NextAttemptAt = now().Add(channelRetryDelay(d.Attempts))
		d.UpdatedAt = now()
		m.mu.Lock()
		_ = m.save("outbound-pending", d.ID, &d)
		m.mu.Unlock()
		_ = m.audit("channel.outbound.retry", map[string]any{"channel_id": d.ChannelID, "delivery_id": d.ID, "attempt": d.Attempts})
		return
	}
	if isAmbiguousChannelError(sendErr) {
		g.markReconciliation(&d, safeChannelError(sendErr))
		return
	}
	g.complete(&d, "failed", safeChannelError(sendErr))
}

func channelRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<uint(attempt-1)) * 5 * time.Second
}

func (g *channelGatewayV170) markReconciliation(d *OutboundDelivery, reason string) {
	m := g.manager
	d.Status = "reconciliation"
	d.LastError = safeChannelError(errors.New(reason))
	d.UpdatedAt = now()
	m.mu.Lock()
	_ = m.save("outbound-pending", d.ID, d)
	m.mu.Unlock()
	_ = m.audit("channel.outbound.reconciliation", map[string]any{"channel_id": d.ChannelID, "delivery_id": d.ID, "task_id": d.TaskID, "attempt": d.Attempts})
}

func (g *channelGatewayV170) complete(d *OutboundDelivery, status, errText string) {
	m := g.manager
	m.mu.RLock()
	var route ChannelRoute
	routeErr := m.read("channel-routes", d.RouteID, &route)
	m.mu.RUnlock()
	digest := ""
	if routeErr == nil {
		digest = senderDigest(route.ReplyTarget)
	}
	t := now()
	r := &OutboundReceipt{ID: d.ID, ChannelID: d.ChannelID, SessionID: d.SessionID, TaskID: d.TaskID, RouteID: d.RouteID, RecipientDigest: digest, Status: status, Attempts: d.Attempts, LastError: safeChannelError(errors.New(errText)), CreatedAt: d.CreatedAt, CompletedAt: &t}
	m.mu.Lock()
	if err := m.save("outbound-receipts", r.ID, r); err == nil {
		if p, pathErr := m.path("outbound-pending", d.ID); pathErr == nil {
			_ = os.Remove(p)
		}
	}
	m.mu.Unlock()
	_ = m.audit("channel.outbound.completed", map[string]any{"channel_id": d.ChannelID, "delivery_id": d.ID, "task_id": d.TaskID, "status": status, "attempts": d.Attempts})
}

func (g *channelGatewayV170) pruneReceipts() {
	m := g.manager
	cutoff := now().Add(-channelReceiptRetention)
	entries, err := os.ReadDir(filepath.Join(m.dir, "outbound-receipts"))
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(m.dir, "outbound-receipts", entry.Name()))
		}
	}
}

type channelHTTPError struct {
	Status int
	Body   string
}

func (e *channelHTTPError) Error() string {
	return fmt.Sprintf("remote channel HTTP %d: %s", e.Status, e.Body)
}

type ambiguousChannelError struct{ err error }

func (e *ambiguousChannelError) Error() string { return e.err.Error() }
func (e *ambiguousChannelError) Unwrap() error { return e.err }

func isRetryableChannelError(err error) bool {
	var he *channelHTTPError
	return errors.As(err, &he) && he.Status == http.StatusTooManyRequests
}

func isAmbiguousChannelError(err error) bool {
	var ae *ambiguousChannelError
	if errors.As(err, &ae) {
		return true
	}
	var he *channelHTTPError
	return errors.As(err, &he) && he.Status >= 500
}

func safeChannelError(err error) string {
	if err == nil {
		return ""
	}
	s := strings.TrimSpace(memory.SanitizeContent(err.Error()))
	if len(s) > 512 {
		s = s[:512]
	}
	return s
}
