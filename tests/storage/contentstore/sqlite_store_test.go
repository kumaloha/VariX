package contentstore

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/model"
)

func TestSQLiteStore_EnablesWALJournalMode(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()
	var mode string
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode error = %v", err)
	}
	if strings.ToLower(mode) != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

func TestSQLiteStore_MarkProcessedAndIsProcessed(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	ok, err := store.IsProcessed(context.Background(), "twitter", "12345")
	if err != nil {
		t.Fatalf("IsProcessed() error = %v", err)
	}
	if ok {
		t.Fatal("expected item to be absent before MarkProcessed")
	}

	err = store.MarkProcessed(context.Background(), types.ProcessedRecord{
		Platform:    "twitter",
		ExternalID:  "12345",
		URL:         "https://x.com/a/status/12345",
		Author:      "alice",
		ProcessedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}

	ok, err = store.IsProcessed(context.Background(), "twitter", "12345")
	if err != nil {
		t.Fatalf("IsProcessed() error = %v", err)
	}
	if !ok {
		t.Fatal("expected item to be present after MarkProcessed")
	}
}

func TestSQLiteStore_RegisterListAndUpdateFollow(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	target := types.FollowTarget{
		Kind:       types.KindNative,
		Platform:   "weibo",
		PlatformID: "123456",
		Locator:    "https://weibo.com/123456",
		URL:        "https://weibo.com/123456",
		FollowedAt: time.Now().UTC(),
	}
	if err := store.RegisterFollow(context.Background(), target); err != nil {
		t.Fatalf("RegisterFollow() error = %v", err)
	}

	items, warnings, err := store.ListFollows(context.Background())
	if err != nil {
		t.Fatalf("ListFollows() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(ListFollows() warnings) = %d, want 0", len(warnings))
	}
	if len(items) != 1 {
		t.Fatalf("len(ListFollows()) = %d, want 1", len(items))
	}

	now := time.Now().UTC()
	if err := store.UpdateFollowPolled(context.Background(), types.KindNative, "weibo", "https://weibo.com/123456", now); err != nil {
		t.Fatalf("UpdateFollowPolled() error = %v", err)
	}

	items, warnings, err = store.ListFollows(context.Background())
	if err != nil {
		t.Fatalf("ListFollows() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(ListFollows() warnings) = %d, want 0", len(warnings))
	}
	if items[0].LastPolledAt.IsZero() {
		t.Fatal("LastPolledAt was not updated")
	}
}

func TestSQLiteStore_RemoveFollow(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	target := types.FollowTarget{
		Kind:       types.KindRSS,
		Platform:   "rss",
		Locator:    "https://feeds.example.test/feed.xml",
		URL:        "https://feeds.example.test/feed.xml",
		FollowedAt: time.Now().UTC(),
	}
	if err := store.RegisterFollow(context.Background(), target); err != nil {
		t.Fatalf("RegisterFollow() error = %v", err)
	}
	if err := store.RemoveFollow(context.Background(), types.KindRSS, "rss", "https://feeds.example.test/feed.xml"); err != nil {
		t.Fatalf("RemoveFollow() error = %v", err)
	}

	items, warnings, err := store.ListFollows(context.Background())
	if err != nil {
		t.Fatalf("ListFollows() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(ListFollows() warnings) = %d, want 0", len(warnings))
	}
	if len(items) != 0 {
		t.Fatalf("len(ListFollows()) = %d, want 0", len(items))
	}
}

func TestSQLiteStore_RegisterFollowRejectsInvalidRecord(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	err = store.RegisterFollow(context.Background(), types.FollowTarget{
		Kind:     types.KindSearch,
		Platform: "twitter",
	})
	if err == nil {
		t.Fatal("expected RegisterFollow() to reject invalid follow target")
	}
}

func TestSQLiteStore_RegisterAuthorSubscriptionWithQueries(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	sub, err := store.RegisterAuthorSubscription(context.Background(), types.AuthorSubscription{
		Platform:   types.PlatformTwitter,
		AuthorName: "Alice",
		PlatformID: "alice",
		ProfileURL: "https://twitter.com/alice",
		Strategy:   types.SubscriptionStrategySearch,
		Status:     "active",
		UpdatedAt:  time.Now().UTC(),
	}, []types.SubscriptionQuery{{
		Provider: "google",
		Query:    "site:x.com/alice/status",
		Priority: 1,
	}})
	if err != nil {
		t.Fatalf("RegisterAuthorSubscription() error = %v", err)
	}
	if sub.ID == 0 {
		t.Fatal("subscription ID was not assigned")
	}

	items, warnings, err := store.ListAuthorSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("ListAuthorSubscriptions() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(warnings) = %d, want 0", len(warnings))
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Strategy != types.SubscriptionStrategySearch || items[0].PlatformID != "alice" {
		t.Fatalf("subscription = %#v", items[0])
	}

	var queryCount int
	if err := store.db.QueryRowContext(context.Background(), `SELECT count(*) FROM subscription_queries WHERE subscription_id = ?`, sub.ID).Scan(&queryCount); err != nil {
		t.Fatalf("count subscription_queries error = %v", err)
	}
	if queryCount != 1 {
		t.Fatalf("queryCount = %d, want 1", queryCount)
	}
}

func TestSQLiteStore_RegisterAuthorSubscriptionUpdatesStableAuthorIdentity(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	first, err := store.RegisterAuthorSubscription(context.Background(), types.AuthorSubscription{
		Platform:   types.PlatformTwitter,
		AuthorName: "Alice",
		PlatformID: "alice",
		ProfileURL: "https://twitter.com/alice",
		Strategy:   types.SubscriptionStrategySearch,
		Status:     "active",
		UpdatedAt:  time.Now().UTC(),
	}, []types.SubscriptionQuery{{
		Provider: "google",
		Query:    "site:x.com/alice/status",
		Priority: 1,
	}})
	if err != nil {
		t.Fatalf("first RegisterAuthorSubscription() error = %v", err)
	}

	second, err := store.RegisterAuthorSubscription(context.Background(), types.AuthorSubscription{
		Platform:   types.PlatformTwitter,
		AuthorName: "Alice Research",
		PlatformID: "alice",
		ProfileURL: "https://twitter.com/alice",
		Strategy:   types.SubscriptionStrategySearch,
		Status:     "active",
		UpdatedAt:  time.Now().UTC(),
	}, []types.SubscriptionQuery{{
		Provider: "google",
		Query:    "site:twitter.com/alice/status",
		Priority: 1,
	}})
	if err != nil {
		t.Fatalf("second RegisterAuthorSubscription() error = %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second.ID = %d, want existing ID %d", second.ID, first.ID)
	}

	items, warnings, err := store.ListAuthorSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("ListAuthorSubscriptions() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(warnings) = %d, want 0", len(warnings))
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].AuthorName != "Alice Research" {
		t.Fatalf("AuthorName = %q, want latest display name", items[0].AuthorName)
	}

	var queryCount int
	if err := store.db.QueryRowContext(context.Background(), `SELECT count(*) FROM subscription_queries WHERE subscription_id = ?`, first.ID).Scan(&queryCount); err != nil {
		t.Fatalf("count subscription_queries error = %v", err)
	}
	if queryCount != 1 {
		t.Fatalf("queryCount = %d, want replacement queries", queryCount)
	}
}

func TestSQLiteStore_RegisterAuthorSubscriptionCanonicalizesTwitterIdentity(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	first, err := store.RegisterAuthorSubscription(context.Background(), types.AuthorSubscription{
		Platform:   types.PlatformTwitter,
		PlatformID: "ElonMusk",
		ProfileURL: "https://x.com/ElonMusk",
		Strategy:   types.SubscriptionStrategySearch,
		Status:     "active",
		UpdatedAt:  time.Now().UTC(),
	}, nil)
	if err != nil {
		t.Fatalf("first RegisterAuthorSubscription() error = %v", err)
	}
	second, err := store.RegisterAuthorSubscription(context.Background(), types.AuthorSubscription{
		Platform:   types.PlatformTwitter,
		PlatformID: "elonmusk",
		ProfileURL: "https://twitter.com/elonmusk",
		Strategy:   types.SubscriptionStrategySearch,
		Status:     "active",
		UpdatedAt:  time.Now().UTC(),
	}, nil)
	if err != nil {
		t.Fatalf("second RegisterAuthorSubscription() error = %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second.ID = %d, want existing ID %d", second.ID, first.ID)
	}
	items, _, err := store.ListAuthorSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("ListAuthorSubscriptions() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].PlatformID != "elonmusk" {
		t.Fatalf("PlatformID = %q, want lowercase canonical id", items[0].PlatformID)
	}
}

func TestSQLiteStore_RegisterAuthorSubscriptionConcurrentSameIdentity(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < cap(errs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.RegisterAuthorSubscription(context.Background(), types.AuthorSubscription{
				Platform:   types.PlatformTwitter,
				PlatformID: "concurrent_author",
				ProfileURL: "https://x.com/concurrent_author",
				Strategy:   types.SubscriptionStrategySearch,
				Status:     "active",
				UpdatedAt:  time.Now().UTC(),
			}, []types.SubscriptionQuery{{
				Provider: "google",
				Query:    "site:x.com/concurrent_author/status",
				Priority: 1,
			}})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("RegisterAuthorSubscription() concurrent error = %v", err)
		}
	}
	items, _, err := store.ListAuthorSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("ListAuthorSubscriptions() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
}

func TestSQLiteStore_RegisterAuthorSubscriptionRejectsNameOnlySearchIdentity(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	_, err = store.RegisterAuthorSubscription(context.Background(), types.AuthorSubscription{
		Platform:   types.PlatformTwitter,
		AuthorName: "Alex Chen",
		Strategy:   types.SubscriptionStrategySearch,
		Status:     "active",
	}, []types.SubscriptionQuery{{
		Provider: "google",
		Query:    `"Alex Chen"`,
	}})
	if err == nil {
		t.Fatal("RegisterAuthorSubscription() error = nil, want name-only search identity rejected")
	}
}

func TestSQLiteStore_MigratesLegacyAuthorSubscriptionDuplicates(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "data", "content.db")
	createLegacyDuplicateAuthorSubscriptionDB(t, path)

	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	items, warnings, err := store.ListAuthorSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("ListAuthorSubscriptions() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(warnings) = %d, want 0", len(warnings))
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want migrated duplicate subscriptions merged", len(items))
	}
	if items[0].AuthorName != "Alice Research" {
		t.Fatalf("AuthorName = %q, want latest display name", items[0].AuthorName)
	}

	var queryCount int
	if err := store.db.QueryRowContext(context.Background(), `SELECT count(*) FROM subscription_queries WHERE subscription_id = ?`, items[0].ID).Scan(&queryCount); err != nil {
		t.Fatalf("count subscription_queries error = %v", err)
	}
	if queryCount != 2 {
		t.Fatalf("queryCount = %d, want merged queries from both legacy rows", queryCount)
	}
	var siteFilter, recencyWindow string
	var priority int
	if err := store.db.QueryRowContext(context.Background(), `SELECT site_filter, recency_window, priority FROM subscription_queries WHERE subscription_id = ? AND provider = 'google' AND query = 'site:x.com/alice/status'`, items[0].ID).Scan(&siteFilter, &recencyWindow, &priority); err != nil {
		t.Fatalf("query migrated metadata error = %v", err)
	}
	if siteFilter != "new-filter" || recencyWindow != "qdr:w" || priority != 7 {
		t.Fatalf("merged query metadata = site_filter=%q recency_window=%q priority=%d, want latest metadata", siteFilter, recencyWindow, priority)
	}

	updated, err := store.RegisterAuthorSubscription(context.Background(), types.AuthorSubscription{
		Platform:   types.PlatformTwitter,
		AuthorName: "Alice Final",
		PlatformID: "alice",
		ProfileURL: "https://twitter.com/alice",
		Strategy:   types.SubscriptionStrategySearch,
		Status:     "active",
		UpdatedAt:  time.Now().UTC(),
	}, []types.SubscriptionQuery{{
		Provider: "google",
		Query:    "site:x.com/alice/status",
		Priority: 1,
	}})
	if err != nil {
		t.Fatalf("RegisterAuthorSubscription() after migration error = %v", err)
	}
	if updated.ID != items[0].ID {
		t.Fatalf("updated.ID = %d, want migrated row ID %d", updated.ID, items[0].ID)
	}

	var indexCount int
	if err := store.db.QueryRowContext(context.Background(), `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_author_subscriptions_platform_identity_key'`).Scan(&indexCount); err != nil {
		t.Fatalf("query unique index error = %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("unique identity index count = %d, want 1", indexCount)
	}
}

func TestSQLiteStore_MigratesLegacyTwitterIdentityCaseDuplicates(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "data", "content.db")
	createLegacyMixedCaseAuthorSubscriptionDB(t, path)

	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	items, _, err := store.ListAuthorSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("ListAuthorSubscriptions() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want mixed-case legacy rows merged", len(items))
	}
	if items[0].PlatformID != "elonmusk" {
		t.Fatalf("platform_id = %q, want canonical lowercase id", items[0].PlatformID)
	}
	if items[0].ProfileURL != "https://twitter.com/elonmusk" {
		t.Fatalf("profile_url = %q, want latest canonical profile", items[0].ProfileURL)
	}
}

func TestSQLiteStore_MigratesDuplicateQueriesKeepsNewestMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "data", "content.db")
	createLegacyDuplicateAuthorSubscriptionDBWithOlderHigherID(t, path)

	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	items, _, err := store.ListAuthorSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("ListAuthorSubscriptions() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want duplicate subscriptions merged", len(items))
	}

	var siteFilter, recencyWindow string
	var priority int
	if err := store.db.QueryRowContext(context.Background(), `SELECT site_filter, recency_window, priority FROM subscription_queries WHERE subscription_id = ? AND provider = 'google' AND query = 'site:x.com/alice/status'`, items[0].ID).Scan(&siteFilter, &recencyWindow, &priority); err != nil {
		t.Fatalf("query migrated metadata error = %v", err)
	}
	if siteFilter != "new-filter" || recencyWindow != "qdr:w" || priority != 7 {
		t.Fatalf("merged query metadata = site_filter=%q recency_window=%q priority=%d, want newest metadata", siteFilter, recencyWindow, priority)
	}
}

func TestSQLiteStore_AuthorSubscriptionsHaveSingleIdentityUniqueIndex(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	rows, err := store.db.QueryContext(context.Background(), `PRAGMA index_list(author_subscriptions)`)
	if err != nil {
		t.Fatalf("PRAGMA index_list error = %v", err)
	}
	defer rows.Close()

	uniqueCount := 0
	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan index_list error = %v", err)
		}
		if unique == 0 {
			continue
		}
		if indexColumns(t, store.db, name) == "platform,identity_key" {
			uniqueCount++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("index_list rows error = %v", err)
	}
	if uniqueCount != 1 {
		t.Fatalf("unique indexes on platform,identity_key = %d, want 1", uniqueCount)
	}
}

func TestSQLiteStore_RegisterAuthorSubscriptionRejectsInvalidRecord(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	_, err = store.RegisterAuthorSubscription(context.Background(), types.AuthorSubscription{
		Platform: types.PlatformTwitter,
		Strategy: types.SubscriptionStrategySearch,
	}, nil)
	if err == nil {
		t.Fatal("expected invalid author subscription to fail")
	}
}

func createLegacyDuplicateAuthorSubscriptionDB(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("ensure legacy db dir error = %v", err)
	}
	db, err := sql.Open("sqlite", sqliteStoreDSN(path))
	if err != nil {
		t.Fatalf("open legacy db error = %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE author_subscriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT NOT NULL,
		author_name TEXT NOT NULL DEFAULT '',
		platform_id TEXT NOT NULL DEFAULT '',
		profile_url TEXT NOT NULL DEFAULT '',
		strategy TEXT NOT NULL,
		rss_url TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		last_checked_at TEXT,
		UNIQUE(platform, platform_id, profile_url, author_name)
	)`); err != nil {
		t.Fatalf("create legacy author_subscriptions error = %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE subscription_queries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		subscription_id INTEGER NOT NULL,
		provider TEXT NOT NULL,
		query TEXT NOT NULL,
		site_filter TEXT NOT NULL DEFAULT '',
		recency_window TEXT NOT NULL DEFAULT '',
		priority INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		UNIQUE(subscription_id, provider, query),
		FOREIGN KEY(subscription_id) REFERENCES author_subscriptions(id) ON DELETE CASCADE
	)`); err != nil {
		t.Fatalf("create legacy subscription_queries error = %v", err)
	}
	oldTime := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	newTime := time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO author_subscriptions(id, platform, author_name, platform_id, profile_url, strategy, status, created_at, updated_at)
		VALUES
		(1, 'twitter', 'Alice', 'alice', 'https://twitter.com/alice', 'search', 'active', ?, ?),
		(2, 'twitter', 'Alice Research', 'alice', 'https://twitter.com/alice', 'search', 'active', ?, ?)`,
		oldTime, oldTime, oldTime, newTime); err != nil {
		t.Fatalf("insert legacy author_subscriptions error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO subscription_queries(subscription_id, provider, query, site_filter, recency_window, priority, created_at)
		VALUES
		(1, 'google', 'site:x.com/alice/status', '', '', 1, ?),
		(2, 'google', 'site:x.com/alice/status', 'new-filter', 'qdr:w', 7, ?),
		(2, 'google', 'site:twitter.com/alice/status', '', '', 2, ?)`,
		oldTime, newTime, newTime); err != nil {
		t.Fatalf("insert legacy subscription_queries error = %v", err)
	}
}

func createLegacyMixedCaseAuthorSubscriptionDB(t *testing.T, path string) {
	t.Helper()
	db := createLegacyAuthorSubscriptionSchema(t, path)
	defer db.Close()

	oldTime := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	newTime := time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO author_subscriptions(id, platform, author_name, platform_id, profile_url, strategy, status, created_at, updated_at)
		VALUES
		(1, 'twitter', 'Elon Musk', 'ElonMusk', 'https://x.com/ElonMusk', 'search', 'active', ?, ?),
		(2, 'twitter', 'Elon Musk', 'elonmusk', 'https://twitter.com/elonmusk', 'search', 'active', ?, ?)`,
		oldTime, oldTime, oldTime, newTime); err != nil {
		t.Fatalf("insert legacy mixed-case author_subscriptions error = %v", err)
	}
}

func createLegacyDuplicateAuthorSubscriptionDBWithOlderHigherID(t *testing.T, path string) {
	t.Helper()
	db := createLegacyAuthorSubscriptionSchema(t, path)
	defer db.Close()

	oldTime := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	newTime := time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO author_subscriptions(id, platform, author_name, platform_id, profile_url, strategy, status, created_at, updated_at)
		VALUES
		(1, 'twitter', 'Alice Research', 'alice', 'https://twitter.com/alice', 'search', 'active', ?, ?),
		(2, 'twitter', 'Alice', 'alice', 'https://twitter.com/alice', 'search', 'active', ?, ?)`,
		oldTime, newTime, oldTime, oldTime); err != nil {
		t.Fatalf("insert legacy reversed author_subscriptions error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO subscription_queries(subscription_id, provider, query, site_filter, recency_window, priority, created_at)
		VALUES
		(1, 'google', 'site:x.com/alice/status', 'new-filter', 'qdr:w', 7, ?),
		(2, 'google', 'site:x.com/alice/status', 'old-filter', 'qdr:d', 1, ?)`,
		newTime, oldTime); err != nil {
		t.Fatalf("insert legacy reversed subscription_queries error = %v", err)
	}
}

func createLegacyAuthorSubscriptionSchema(t *testing.T, path string) *sql.DB {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("ensure legacy db dir error = %v", err)
	}
	db, err := sql.Open("sqlite", sqliteStoreDSN(path))
	if err != nil {
		t.Fatalf("open legacy db error = %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE author_subscriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT NOT NULL,
		author_name TEXT NOT NULL DEFAULT '',
		platform_id TEXT NOT NULL DEFAULT '',
		profile_url TEXT NOT NULL DEFAULT '',
		strategy TEXT NOT NULL,
		rss_url TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		last_checked_at TEXT,
		UNIQUE(platform, platform_id, profile_url, author_name)
	)`); err != nil {
		db.Close()
		t.Fatalf("create legacy author_subscriptions error = %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE subscription_queries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		subscription_id INTEGER NOT NULL,
		provider TEXT NOT NULL,
		query TEXT NOT NULL,
		site_filter TEXT NOT NULL DEFAULT '',
		recency_window TEXT NOT NULL DEFAULT '',
		priority INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		UNIQUE(subscription_id, provider, query),
		FOREIGN KEY(subscription_id) REFERENCES author_subscriptions(id) ON DELETE CASCADE
	)`); err != nil {
		db.Close()
		t.Fatalf("create legacy subscription_queries error = %v", err)
	}
	return db
}

func indexColumns(t *testing.T, db *sql.DB, indexName string) string {
	t.Helper()
	rows, err := db.Query(`PRAGMA index_info(` + indexName + `)`)
	if err != nil {
		t.Fatalf("PRAGMA index_info(%s) error = %v", indexName, err)
	}
	defer rows.Close()
	columns := make([]string, 0)
	for rows.Next() {
		var seqno, cid int
		var name string
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			t.Fatalf("scan index_info error = %v", err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("index_info rows error = %v", err)
	}
	return strings.Join(columns, ",")
}

func TestSQLiteStore_RecordPollReportPersistsRunAndTargets(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	report := types.PollReport{
		StartedAt:         time.Now().UTC().Add(-time.Minute),
		FinishedAt:        time.Now().UTC(),
		TargetCount:       2,
		DiscoveredCount:   3,
		FetchedCount:      2,
		SkippedCount:      1,
		StoreWarningCount: 0,
		PollWarningCount:  1,
		Targets: []types.TargetPollReport{
			{Target: "search:twitter:nvda", DiscoveredCount: 2, FetchedCount: 1, SkippedCount: 1, WarningCount: 0, Status: "ok"},
			{Target: "rss:rss:https://example.com/feed.xml", DiscoveredCount: 1, FetchedCount: 1, SkippedCount: 0, WarningCount: 1, Status: "warning", ErrorDetail: "hydrate failed"},
		},
	}

	if err := store.RecordPollReport(context.Background(), report); err != nil {
		t.Fatalf("RecordPollReport() error = %v", err)
	}

	var runCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM poll_runs`).Scan(&runCount); err != nil {
		t.Fatalf("QueryRow(poll_runs) error = %v", err)
	}
	if runCount != 1 {
		t.Fatalf("poll_runs count = %d, want 1", runCount)
	}

	var targetCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM poll_target_runs`).Scan(&targetCount); err != nil {
		t.Fatalf("QueryRow(poll_target_runs) error = %v", err)
	}
	if targetCount != 2 {
		t.Fatalf("poll_target_runs count = %d, want 2", targetCount)
	}
}

func testCompileGraphNode(id string, kind model.NodeKind, text string) model.GraphNode {
	return model.GraphNode{
		ID:        id,
		Kind:      kind,
		Text:      text,
		ValidFrom: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
		ValidTo:   time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
	}
}

func TestSQLiteStore_UpsertAndGetCompiledOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "twitter:123",
		Source:         "twitter",
		ExternalID:     "123",
		RootExternalID: "100",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary text",
			Drivers: []string{"driver"},
			Targets: []string{"target"},
			Brief: []model.BriefItem{{
				Category: "portfolio",
				Kind:     "list",
				Claim:    "Apple remains a core holding.",
				Entities: []string{"Apple"},
			}},
			Ledger: model.Ledger{Items: []model.LedgerItem{{
				ID:        "ledger-001",
				Category:  "portfolio",
				Kind:      "list",
				Claim:     "Apple remains a core holding.",
				Entities:  []string{"Apple"},
				SourceIDs: []string{"semantic-001"},
			}}},
			CoverageAudit: model.CoverageAudit{
				MissingCategories: []string{"portfolio"},
				OmittedLedgerIDs:  []string{"ledger-002"},
			},
			TransmissionPaths: []model.TransmissionPath{{Driver: "driver", Target: "target", Steps: []string{"step"}}},
			Branches: []model.Branch{{
				ID:                "s1",
				Level:             "primary",
				Policy:            "forecast_inference",
				Thesis:            "branch thesis",
				Anchors:           []string{"anchor"},
				BranchDrivers:     []string{"branch driver"},
				Drivers:           []string{"driver"},
				Targets:           []string{"target"},
				TransmissionPaths: []model.TransmissionPath{{Driver: "driver", Target: "target", Steps: []string{"step"}}},
			}},
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					testCompileGraphNode("n1", model.NodeFact, "fact"),
					testCompileGraphNode("n2", model.NodeConclusion, "conclusion"),
				},
				Edges: []model.GraphEdge{
					{From: "n1", To: "n2", Kind: model.EdgePositive},
				},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Topics:     []string{"topic-a"},
			Confidence: "medium",
			AuthorValidation: model.AuthorValidation{
				Version: "author_claim_validation",
				Summary: model.AuthorValidationSummary{
					Verdict:         "mixed",
					SupportedClaims: 1,
				},
				ClaimChecks: []model.AuthorClaimCheck{{
					ClaimID: "claim-001",
					Text:    "driver",
					Status:  model.AuthorClaimSupported,
				}},
			},
		},
		CompiledAt: time.Now().UTC(),
	}

	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	got, err := store.GetCompiledOutput(context.Background(), "twitter", "123")
	if err != nil {
		t.Fatalf("GetCompiledOutput() error = %v", err)
	}
	if got.Model != "qwen3.6-plus" {
		t.Fatalf("Model = %q", got.Model)
	}
	if got.Output.Summary != "summary text" {
		t.Fatalf("Summary = %q", got.Output.Summary)
	}
	if got.RootExternalID != "100" {
		t.Fatalf("RootExternalID = %q", got.RootExternalID)
	}
	if len(got.Output.Branches) != 1 {
		t.Fatalf("Branches = %#v, want persisted branch", got.Output.Branches)
	}
	if got.Output.Branches[0].Thesis != "branch thesis" {
		t.Fatalf("Branch thesis = %q", got.Output.Branches[0].Thesis)
	}
	if len(got.Output.Branches[0].Anchors) != 1 || got.Output.Branches[0].Anchors[0] != "anchor" {
		t.Fatalf("Branch anchors = %#v", got.Output.Branches[0].Anchors)
	}
	if len(got.Output.Branches[0].BranchDrivers) != 1 || got.Output.Branches[0].BranchDrivers[0] != "branch driver" {
		t.Fatalf("Branch drivers = %#v", got.Output.Branches[0].BranchDrivers)
	}
	if len(got.Output.Branches[0].TransmissionPaths) != 1 || got.Output.Branches[0].TransmissionPaths[0].Target != "target" {
		t.Fatalf("Branch transmission paths = %#v", got.Output.Branches[0].TransmissionPaths)
	}
	if len(got.Output.Brief) != 1 || got.Output.Brief[0].Category != "portfolio" || got.Output.Brief[0].Entities[0] != "Apple" {
		t.Fatalf("Brief = %#v, want persisted brief", got.Output.Brief)
	}
	if len(got.Output.Ledger.Items) != 1 || got.Output.Ledger.Items[0].Category != "portfolio" || got.Output.Ledger.Items[0].Entities[0] != "Apple" {
		t.Fatalf("Ledger = %#v, want persisted ledger", got.Output.Ledger)
	}
	if len(got.Output.CoverageAudit.MissingCategories) != 1 || got.Output.CoverageAudit.MissingCategories[0] != "portfolio" {
		t.Fatalf("CoverageAudit = %#v, want persisted coverage audit", got.Output.CoverageAudit)
	}
	if got.Output.AuthorValidation.Summary.Verdict != "mixed" || len(got.Output.AuthorValidation.ClaimChecks) != 1 {
		t.Fatalf("AuthorValidation = %#v, want persisted author validation", got.Output.AuthorValidation)
	}
}

func TestSQLiteStore_UpsertCompiledOutputBridgesLegacyEmbeddedVerificationToVerificationTable(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "twitter:legacy-verify",
		Source:         "twitter",
		ExternalID:     "legacy-verify",
		RootExternalID: "legacy-root",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary text",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					testCompileGraphNode("n1", model.NodeFact, "fact"),
					testCompileGraphNode("n2", model.NodeConclusion, "conclusion"),
				},
				Edges: []model.GraphEdge{
					{From: "n1", To: "n2", Kind: model.EdgePositive},
				},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
			Verification: model.Verification{
				Model: "verify-model",
				FactChecks: []model.FactCheck{{
					NodeID: "n1",
					Status: model.FactStatusClearlyTrue,
					Reason: "supported",
				}},
				VerifiedAt: time.Now().UTC(),
			},
		},
		CompiledAt: time.Now().UTC(),
	}

	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}

	gotCompile, err := store.GetCompiledOutput(context.Background(), "twitter", "legacy-verify")
	if err != nil {
		t.Fatalf("GetCompiledOutput() error = %v", err)
	}
	if !gotCompile.Output.Verification.IsZero() {
		t.Fatalf("compiled output verification = %#v, want compile store decoupled from verification payload", gotCompile.Output.Verification)
	}

	gotVerify, err := store.GetVerificationResult(context.Background(), "twitter", "legacy-verify")
	if err != nil {
		t.Fatalf("GetVerificationResult() error = %v", err)
	}
	if gotVerify.Model != "verify-model" {
		t.Fatalf("verification model = %q, want verify-model", gotVerify.Model)
	}
	if len(gotVerify.Verification.FactChecks) != 1 || gotVerify.Verification.FactChecks[0].NodeID != "n1" {
		t.Fatalf("verification = %#v, want bridged fact check", gotVerify.Verification)
	}
}
