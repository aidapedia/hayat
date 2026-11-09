// Package audit records who changed what. Every admin mutation writes one row,
// in the same transaction as the change.
package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/aidapedia/hayat/internal/store"
)

// Actor names who acted: "user:<id>", "system", or "setup".
func UserActor(id uuid.UUID) string { return "user:" + id.String() }

const System = "system"

func Write(ctx context.Context, db store.DB, region, actor, action, target string, detail map[string]any) error {
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx,
		`INSERT INTO audit_log (region, id, actor, action, target, detail) VALUES ($1, $2, $3, $4, $5, $6)`,
		region, store.NewID(), actor, action, target, raw)
	return err
}

type Event struct {
	ID     uuid.UUID
	At     time.Time
	Actor  string
	Action string
	Target string
	Detail map[string]any
}

// List returns events newest first, before the cursor time when one is given.
func List(ctx context.Context, db store.DB, before time.Time, limit int) ([]Event, error) {
	if before.IsZero() {
		before = time.Now().Add(time.Minute)
	}
	rows, err := db.Query(ctx,
		`SELECT id, at, actor, action, target, detail FROM audit_log WHERE at < $1 ORDER BY at DESC LIMIT $2`,
		before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var raw []byte
		if err := rows.Scan(&e.ID, &e.At, &e.Actor, &e.Action, &e.Target, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &e.Detail)
		out = append(out, e)
	}
	return out, rows.Err()
}
