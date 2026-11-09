// Package worker is the `worker` role: background jobs that must run once per
// region, not once per pod.
//
// Pods compete for a lease row; only the holder works. The lease expires on its
// own if the holder dies, so a standby takes over within leaseTTL.
package worker

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/mail"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
	"github.com/aidapedia/hayat/internal/store"
)

const (
	leaseTTL     = 30 * time.Second
	tick         = 5 * time.Second
	cleanupEvery = 10 * time.Minute
	rotateCheck  = time.Hour
	maxAttempts  = 10
)

type Worker struct {
	cfg    config.Config
	store  *store.Store
	sender mail.Sender
	keys   *hoidc.KeyRing
	log    *slog.Logger
	holder string
}

func New(cfg config.Config, s *store.Store, sender mail.Sender, keys *hoidc.KeyRing, log *slog.Logger) *Worker {
	host, _ := os.Hostname()
	return &Worker{cfg: cfg, store: s, sender: sender, keys: keys, log: log, holder: host + "/" + uuid.NewString()[:8]}
}

// Run works until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(tick)
	defer t.Stop()
	var lastCleanup, lastRotateCheck time.Time
	for {
		if w.lease(ctx) {
			w.sendMail(ctx)
			if time.Since(lastCleanup) > cleanupEvery {
				w.cleanup(ctx)
				lastCleanup = time.Now()
			}
			if time.Since(lastRotateCheck) > rotateCheck {
				if rotated, err := w.keys.RotateIfDue(ctx, w.cfg.KeyRotation); err != nil {
					w.log.Error("key rotation check", "err", err)
				} else if rotated {
					w.log.Info("rotated signing keys")
				}
				lastRotateCheck = time.Now()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// lease takes or renews this region's lease and reports whether we hold it.
func (w *Worker) lease(ctx context.Context) bool {
	var holder string
	err := w.store.Pool.QueryRow(ctx, `INSERT INTO worker_leases (name, holder, expires_at) VALUES ($1, $2, now() + $3::interval)
		ON CONFLICT (name) DO UPDATE SET holder = EXCLUDED.holder, expires_at = EXCLUDED.expires_at
		WHERE worker_leases.holder = EXCLUDED.holder OR worker_leases.expires_at < now()
		RETURNING holder`, "worker:"+w.cfg.Region, w.holder, leaseTTL.String()).Scan(&holder)
	return err == nil && holder == w.holder
}

type outboxRow struct {
	id       uuid.UUID
	to       string
	subject  string
	body     string
	attempts int
}

func (w *Worker) sendMail(ctx context.Context) {
	rows, err := w.store.Pool.Query(ctx, `SELECT id, to_addr, subject, body_text, attempts FROM email_outbox
		WHERE region = $1 AND sent_at IS NULL AND send_after <= now() AND attempts < $2
		ORDER BY created_at LIMIT 20`, w.cfg.Region, maxAttempts)
	if err != nil {
		w.log.Error("read outbox", "err", err)
		return
	}
	var batch []outboxRow
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.id, &r.to, &r.subject, &r.body, &r.attempts); err == nil {
			batch = append(batch, r)
		}
	}
	rows.Close()

	for _, r := range batch {
		sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := w.sender.Send(sendCtx, r.to, r.subject, r.body)
		cancel()
		if err != nil {
			// Back off 1, 4, 9 … minutes, up to maxAttempts.
			backoff := time.Duration((r.attempts+1)*(r.attempts+1)) * time.Minute
			_, _ = w.store.Pool.Exec(ctx, `UPDATE email_outbox SET attempts = attempts + 1, last_error = $3, send_after = now() + $4::interval
				WHERE region = $1 AND id = $2`, w.cfg.Region, r.id, err.Error(), backoff.String())
			w.log.Warn("send email", "to", r.to, "attempt", r.attempts+1, "err", err)
			continue
		}
		_, _ = w.store.Pool.Exec(ctx, `UPDATE email_outbox SET sent_at = now(), attempts = attempts + 1 WHERE region = $1 AND id = $2`, w.cfg.Region, r.id)
	}
}

// cleanup deletes rows nothing can use any more.
func (w *Worker) cleanup(ctx context.Context) {
	for _, q := range []string{
		`DELETE FROM oidc_auth_requests WHERE region = $1 AND expires_at < now() - interval '1 hour'`,
		`DELETE FROM email_tokens WHERE region = $1 AND (used_at IS NOT NULL OR expires_at < now()) AND created_at < now() - interval '7 days'`,
		`DELETE FROM sessions WHERE region = $1 AND (revoked_at IS NOT NULL OR expires_at < now()) AND created_at < now() - interval '30 days'`,
		`DELETE FROM refresh_tokens WHERE region = $1 AND (revoked_at IS NOT NULL OR rotated_at IS NOT NULL OR expires_at < now()) AND created_at < now() - interval '30 days'`,
		`DELETE FROM email_outbox WHERE region = $1 AND sent_at < now() - interval '30 days'`,
		`DELETE FROM policy_events WHERE region = $1 AND created_at < now() - interval '1 day'`,
	} {
		if _, err := w.store.Pool.Exec(ctx, q, w.cfg.Region); err != nil {
			w.log.Error("cleanup", "err", err)
		}
	}
	if _, err := w.store.Pool.Exec(ctx, `DELETE FROM oidc_code_refs WHERE expires_at < now() - interval '1 hour'`); err != nil {
		w.log.Error("cleanup codes", "err", err)
	}
}
