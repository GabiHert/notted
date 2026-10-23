package store

import (
	"context"
	"encoding/json"

	"github.com/bhlabz/notted/internal/domain"
)

const (
	keyProfile      = "profile"
	keyCheckoutPath = "checkout_path"
	keyProjectRoots = "project_roots"
)

func (s *Store) GetSettings(ctx context.Context) (domain.Settings, error) {
	out := domain.Settings{ProjectRoots: map[string]string{}}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return out, err
		}
		switch k {
		case keyProfile:
			out.Profile = v
		case keyCheckoutPath:
			out.CheckoutPath = v
		case keyProjectRoots:
			roots := map[string]string{}
			if err := json.Unmarshal([]byte(v), &roots); err != nil {
				return out, err
			}
			if roots != nil {
				out.ProjectRoots = roots
			}
		}
	}
	return out, rows.Err()
}

func (s *Store) SaveSettings(ctx context.Context, st domain.Settings) error {
	roots := st.ProjectRoots
	if roots == nil {
		roots = map[string]string{}
	}
	rootsJSON, err := json.Marshal(roots)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for k, v := range map[string]string{keyProfile: st.Profile, keyCheckoutPath: st.CheckoutPath, keyProjectRoots: string(rootsJSON)} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}
