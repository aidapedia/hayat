// Package mail queues email in the database and delivers it from the worker.
//
// Queuing in the same transaction as the change that caused it means a
// verification email is sent if and only if the account it verifies exists.
package mail

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"net/url"
	"strings"
	"time"

	"github.com/aidapedia/hayat/internal/store"
)

// Enqueue adds a message to the outbox inside the caller's transaction.
func Enqueue(ctx context.Context, db store.DB, region, to, subject, body string) error {
	_, err := db.Exec(ctx,
		`INSERT INTO email_outbox (region, id, to_addr, subject, body_text) VALUES ($1, $2, $3, $4, $5)`,
		region, store.NewID(), to, subject, body)
	return err
}

// Sender delivers one message.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// NewSender returns an SMTP sender for smtp://user:pass@host:port, or a sender
// that logs the message when the URL is empty, which is what development wants:
// the verification link shows up in the log.
func NewSender(rawURL, from string, log *slog.Logger) (Sender, error) {
	if rawURL == "" {
		return logSender{log: log}, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "smtp" && u.Scheme != "smtps") {
		return nil, fmt.Errorf("mail: HAYAT_SMTP_URL must look like smtp://user:pass@host:587")
	}
	return &smtpSender{u: u, from: from}, nil
}

type logSender struct{ log *slog.Logger }

func (l logSender) Send(_ context.Context, to, subject, body string) error {
	l.log.Info("email (not sent: HAYAT_SMTP_URL is empty)", "to", to, "subject", subject, "body", body)
	return nil
}

type smtpSender struct {
	u    *url.URL
	from string
}

func (s *smtpSender) Send(_ context.Context, to, subject, body string) error {
	host := s.u.Hostname()
	addr := net.JoinHostPort(host, s.u.Port())
	var auth smtp.Auth
	if s.u.User != nil {
		pass, _ := s.u.User.Password()
		auth = smtp.PlainAuth("", s.u.User.Username(), pass, host)
	}
	msg := strings.Join([]string{
		"From: " + s.from,
		"To: " + to,
		"Subject: " + subject,
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		body,
	}, "\r\n")
	return smtp.SendMail(addr, auth, addressOnly(s.from), []string{to}, []byte(msg))
}

func addressOnly(from string) string {
	if i := strings.LastIndex(from, "<"); i >= 0 {
		return strings.TrimSuffix(from[i+1:], ">")
	}
	return from
}
