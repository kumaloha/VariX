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
	identityKey := authorSubscriptionIdentityKey(sub)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return types.AuthorSubscription{}, err
	}
	defer tx.Rollback()

	var createdAt string
	err = tx.QueryRowContext(
		ctx,
		`INSERT INTO author_subscriptions(platform, author_name, identity_key, platform_id, profile_url, strategy, rss_url, status, created_at, updated_at, last_checked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(platform, identity_key) DO UPDATE SET
		   author_name = excluded.author_name,
		   platform_id = excluded.platform_id,
		   profile_url = excluded.profile_url,
		   strategy = excluded.strategy,
		   rss_url = excluded.rss_url,
		   status = excluded.status,
		   updated_at = excluded.updated_at,
		   last_checked_at = excluded.last_checked_at
		 RETURNING id, created_at`,
		string(sub.Platform),
		sub.AuthorName,
		identityKey,
		sub.PlatformID,
		sub.ProfileURL,
		string(sub.Strategy),
		sub.RSSURL,
		sub.Status,
		sub.CreatedAt.UTC().Format(time.RFC3339Nano),
		sub.UpdatedAt.UTC().Format(time.RFC3339Nano),
		formatSQLiteTime(sub.LastCheckedAt),
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
	sub.PlatformID = canonicalAuthorPlatformID(sub.Platform, strings.TrimSpace(sub.PlatformID))
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
	if sub.Strategy != types.SubscriptionStrategyRSS && sub.Strategy != types.SubscriptionStrategySearch && sub.Strategy != types.SubscriptionStrategyNative {
		return false
	}
	return sub.PlatformID != "" || sub.ProfileURL != "" || sub.RSSURL != ""
}

func canonicalAuthorPlatformID(platform types.Platform, id string) string {
	id = strings.Trim(strings.TrimSpace(id), "@")
	if platform == types.PlatformTwitter {
		return strings.ToLower(id)
	}
	return id
}

func authorSubscriptionIdentityKey(sub types.AuthorSubscription) string {
	if sub.PlatformID != "" {
		return "id:" + sub.PlatformID
	}
	if sub.ProfileURL != "" {
		return "profile:" + normalizeAuthorIdentityURL(sub.ProfileURL)
	}
	if sub.RSSURL != "" {
		return "rss:" + normalizeAuthorIdentityURL(sub.RSSURL)
	}
	return ""
}

func normalizeAuthorIdentityURL(raw string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(raw)), "/")
}

func (s *SQLiteStore) backfillAuthorSubscriptionIdentityKeys() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id, platform, author_name, platform_id, profile_url, strategy, rss_url, status, created_at, updated_at, last_checked_at
		FROM author_subscriptions
		WHERE identity_key = ''`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type update struct {
		sub         types.AuthorSubscription
		identityKey string
	}
	updates := make([]update, 0)
	for rows.Next() {
		sub, err := scanAuthorSubscription(rows)
		if err != nil {
			return err
		}
		sub = normalizeAuthorSubscription(sub)
		identityKey := authorSubscriptionIdentityKey(sub)
		if identityKey == "" {
			identityKey = fmt.Sprintf("legacy:%d", sub.ID)
		}
		updates = append(updates, update{
			sub:         sub,
			identityKey: identityKey,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, update := range updates {
		if _, err := tx.Exec(`UPDATE author_subscriptions
			SET author_name = ?, platform_id = ?, profile_url = ?, strategy = ?, rss_url = ?, status = ?, identity_key = ?
			WHERE id = ?`,
			update.sub.AuthorName,
			update.sub.PlatformID,
			update.sub.ProfileURL,
			string(update.sub.Strategy),
			update.sub.RSSURL,
			update.sub.Status,
			update.identityKey,
			update.sub.ID,
		); err != nil {
			return err
		}
	}
	if err := mergeDuplicateAuthorSubscriptions(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_author_subscriptions_platform_identity_key
		ON author_subscriptions(platform, identity_key)`); err != nil {
		return err
	}
	return tx.Commit()
}

func mergeDuplicateAuthorSubscriptions(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT platform, identity_key
		FROM author_subscriptions
		WHERE identity_key != ''
		GROUP BY platform, identity_key
		HAVING count(*) > 1`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type duplicateGroup struct {
		platform    string
		identityKey string
	}
	groups := make([]duplicateGroup, 0)
	for rows.Next() {
		var group duplicateGroup
		if err := rows.Scan(&group.platform, &group.identityKey); err != nil {
			return err
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, group := range groups {
		if err := mergeAuthorSubscriptionGroup(tx, group.platform, group.identityKey); err != nil {
			return err
		}
	}
	return nil
}

type authorSubscriptionMergeRow struct {
	id             int64
	authorName     string
	platformID     string
	profileURL     string
	strategy       string
	rssURL         string
	status         string
	createdAt      string
	updatedAt      string
	lastCheckedAt  sql.NullString
	updatedAtValue time.Time
}

func mergeAuthorSubscriptionGroup(tx *sql.Tx, platform, identityKey string) error {
	rows, err := tx.Query(`SELECT id, author_name, platform_id, profile_url, strategy, rss_url, status, created_at, updated_at, last_checked_at
		FROM author_subscriptions
		WHERE platform = ? AND identity_key = ?
		ORDER BY id ASC`, platform, identityKey)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := make([]authorSubscriptionMergeRow, 0)
	for rows.Next() {
		var item authorSubscriptionMergeRow
		if err := rows.Scan(
			&item.id,
			&item.authorName,
			&item.platformID,
			&item.profileURL,
			&item.strategy,
			&item.rssURL,
			&item.status,
			&item.createdAt,
			&item.updatedAt,
			&item.lastCheckedAt,
		); err != nil {
			return err
		}
		item.updatedAtValue = parseSQLiteTime(item.updatedAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(items) < 2 {
		return nil
	}

	keeper := items[0]
	preferred := items[0]
	for _, item := range items[1:] {
		if item.updatedAtValue.After(preferred.updatedAtValue) {
			preferred = item
		}
	}
	if err := mergeSubscriptionQueriesForGroup(tx, keeper.id, items); err != nil {
		return err
	}
	for _, item := range items[1:] {
		if _, err := tx.Exec(`DELETE FROM subscription_queries WHERE subscription_id = ?`, item.id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM author_subscriptions WHERE id = ?`, item.id); err != nil {
			return err
		}
	}

	_, err = tx.Exec(`UPDATE author_subscriptions
		SET author_name = ?, platform_id = ?, profile_url = ?, strategy = ?, rss_url = ?, status = ?, updated_at = ?, last_checked_at = ?
		WHERE id = ?`,
		preferred.authorName,
		preferred.platformID,
		preferred.profileURL,
		preferred.strategy,
		preferred.rssURL,
		preferred.status,
		preferred.updatedAt,
		preferred.lastCheckedAt,
		keeper.id,
	)
	return err
}

type subscriptionQueryMergeRow struct {
	provider      string
	query         string
	siteFilter    string
	recencyWindow string
	priority      int
	createdAt     string
	recency       time.Time
	sourceID      int64
}

func mergeSubscriptionQueriesForGroup(tx *sql.Tx, keeperID int64, items []authorSubscriptionMergeRow) error {
	best := make(map[string]subscriptionQueryMergeRow)
	order := make([]string, 0)
	for _, item := range items {
		rows, err := tx.Query(`SELECT provider, query, site_filter, recency_window, priority, created_at
			FROM subscription_queries
			WHERE subscription_id = ?`, item.id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var query subscriptionQueryMergeRow
			if err := rows.Scan(&query.provider, &query.query, &query.siteFilter, &query.recencyWindow, &query.priority, &query.createdAt); err != nil {
				rows.Close()
				return err
			}
			query.sourceID = item.id
			query.recency = parseSQLiteTime(query.createdAt)
			if item.updatedAtValue.After(query.recency) {
				query.recency = item.updatedAtValue
			}
			key := query.provider + "\x00" + query.query
			existing, ok := best[key]
			if !ok {
				order = append(order, key)
				best[key] = query
				continue
			}
			if query.recency.After(existing.recency) || (query.recency.Equal(existing.recency) && query.sourceID > existing.sourceID) {
				best[key] = query
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`DELETE FROM subscription_queries WHERE subscription_id = ?`, keeperID); err != nil {
		return err
	}
	for _, key := range order {
		query := best[key]
		if _, err := tx.Exec(`INSERT INTO subscription_queries(subscription_id, provider, query, site_filter, recency_window, priority, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			keeperID,
			query.provider,
			query.query,
			query.siteFilter,
			query.recencyWindow,
			query.priority,
			query.createdAt,
		); err != nil {
			return err
		}
	}
	return nil
}
