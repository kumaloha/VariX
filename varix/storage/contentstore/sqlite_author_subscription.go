package contentstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func (s *SQLiteStore) RegisterAuthorSubscription(ctx context.Context, sub types.AuthorSubscription, queries []types.SubscriptionQuery) (types.AuthorSubscription, error) {
	sub = normalizeAuthorSubscription(sub)
	if !isValidAuthorSubscription(sub) {
		return types.AuthorSubscription{}, fmt.Errorf("invalid author subscription")
	}
	now := normalizeRecordedTime(sub.UpdatedAt)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	sub.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return types.AuthorSubscription{}, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO author_subscriptions(platform, author_name, platform_id, profile_url, strategy, rss_url, status, created_at, updated_at, last_checked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(platform, platform_id, profile_url, author_name) DO UPDATE SET
		   author_name = excluded.author_name,
		   strategy = excluded.strategy,
		   rss_url = excluded.rss_url,
		   status = excluded.status,
		   updated_at = excluded.updated_at,
		   last_checked_at = excluded.last_checked_at`,
		string(sub.Platform),
		sub.AuthorName,
		sub.PlatformID,
		sub.ProfileURL,
		string(sub.Strategy),
		sub.RSSURL,
		sub.Status,
		sub.CreatedAt.UTC().Format(time.RFC3339Nano),
		sub.UpdatedAt.UTC().Format(time.RFC3339Nano),
		formatSQLiteTime(sub.LastCheckedAt),
	)
	if err != nil {
		return types.AuthorSubscription{}, err
	}
	var createdAt string
	err = tx.QueryRowContext(
		ctx,
		`SELECT id, created_at
		 FROM author_subscriptions
		 WHERE platform = ? AND platform_id = ? AND profile_url = ? AND author_name = ?`,
		string(sub.Platform),
		sub.PlatformID,
		sub.ProfileURL,
		sub.AuthorName,
	).Scan(&sub.ID, &createdAt)
	if err != nil {
		return types.AuthorSubscription{}, err
	}
	sub.CreatedAt = parseSQLiteTime(createdAt)

	if _, err := tx.ExecContext(ctx, `DELETE FROM subscription_queries WHERE subscription_id = ?`, sub.ID); err != nil {
		return types.AuthorSubscription{}, err
	}
	if len(queries) > 0 {
		for i, query := range queries {
			query.Query = strings.Join(strings.Fields(query.Query), " ")
			query.Provider = strings.TrimSpace(query.Provider)
			query.SiteFilter = strings.TrimSpace(query.SiteFilter)
			query.RecencyWindow = strings.TrimSpace(query.RecencyWindow)
			if query.Provider == "" || query.Query == "" {
				return types.AuthorSubscription{}, fmt.Errorf("invalid subscription query")
			}
			if query.Priority == 0 {
				query.Priority = i + 1
			}
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO subscription_queries(subscription_id, provider, query, site_filter, recency_window, priority, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				sub.ID,
				query.Provider,
				query.Query,
				query.SiteFilter,
				query.RecencyWindow,
				query.Priority,
				now.UTC().Format(time.RFC3339Nano),
			); err != nil {
				return types.AuthorSubscription{}, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return types.AuthorSubscription{}, err
	}
	return sub, nil
}

func (s *SQLiteStore) ListAuthorSubscriptions(ctx context.Context) ([]types.AuthorSubscription, []ScanWarning, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, platform, author_name, platform_id, profile_url, strategy, rss_url, status, created_at, updated_at, last_checked_at
		 FROM author_subscriptions
		 ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	items := make([]types.AuthorSubscription, 0)
	for rows.Next() {
		item, err := scanAuthorSubscription(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return items, nil, nil
}

func scanAuthorSubscription(rows interface {
	Scan(dest ...any) error
}) (types.AuthorSubscription, error) {
	var item types.AuthorSubscription
	var platform, strategy, createdAt, updatedAt string
	var lastChecked sql.NullString
	if err := rows.Scan(
		&item.ID,
		&platform,
		&item.AuthorName,
		&item.PlatformID,
		&item.ProfileURL,
		&strategy,
		&item.RSSURL,
		&item.Status,
		&createdAt,
		&updatedAt,
		&lastChecked,
	); err != nil {
		return types.AuthorSubscription{}, err
	}
	item.Platform = types.Platform(platform)
	item.Strategy = types.SubscriptionStrategy(strategy)
	item.CreatedAt = parseSQLiteTime(createdAt)
	item.UpdatedAt = parseSQLiteTime(updatedAt)
	if lastChecked.Valid {
		item.LastCheckedAt = parseSQLiteTime(lastChecked.String)
	}
	return item, nil
}

func normalizeAuthorSubscription(sub types.AuthorSubscription) types.AuthorSubscription {
	sub.Platform = types.Platform(strings.TrimSpace(string(sub.Platform)))
	sub.AuthorName = strings.Join(strings.Fields(sub.AuthorName), " ")
	sub.PlatformID = strings.TrimSpace(sub.PlatformID)
	sub.ProfileURL = strings.TrimSpace(sub.ProfileURL)
	sub.RSSURL = strings.TrimSpace(sub.RSSURL)
	sub.Status = strings.TrimSpace(sub.Status)
	if sub.Status == "" {
		sub.Status = "active"
	}
	return sub
}

func isValidAuthorSubscription(sub types.AuthorSubscription) bool {
	if strings.TrimSpace(string(sub.Platform)) == "" || strings.TrimSpace(string(sub.Strategy)) == "" {
		return false
	}
	if sub.Strategy != types.SubscriptionStrategyRSS && sub.Strategy != types.SubscriptionStrategySearch {
		return false
	}
	return sub.PlatformID != "" || sub.ProfileURL != "" || sub.AuthorName != ""
}
