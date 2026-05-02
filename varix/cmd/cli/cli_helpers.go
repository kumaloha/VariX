package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func setRawURLFromArg(fs *flag.FlagSet, rawURL *string) {
	if rawURL == nil || strings.TrimSpace(*rawURL) != "" || fs == nil || fs.NArg() == 0 {
		return
	}
	*rawURL = fs.Arg(0)
}

func resolveContentTarget(ctx context.Context, app *ingest.Runtime, rawURL, platform, externalID string) (string, string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return strings.TrimSpace(platform), strings.TrimSpace(externalID), nil
	}
	parsed, err := app.Dispatcher.ParseURL(ctx, rawURL)
	if err != nil {
		return "", "", err
	}
	return string(parsed.Platform), parsed.PlatformID, nil
}

func openRuntimeStore(projectRoot string) (*ingest.Runtime, *contentstore.SQLiteStore, error) {
	app, err := newIngestRuntime(projectRoot)
	if err != nil {
		return nil, nil, err
	}
	if app.Store != nil {
		return app, app.Store, nil
	}
	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		return nil, nil, err
	}
	return app, store, nil
}

func openStore(projectRoot string) (*contentstore.SQLiteStore, error) {
	_, store, err := openRuntimeStore(projectRoot)
	return store, err
}

func writeJSON(stdout, stderr io.Writer, value any) int {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	_, _ = io.WriteString(stdout, string(payload))
	_, _ = io.WriteString(stdout, "\n")
	return 0
}

func writeErr(stderr io.Writer, err error) {
	if err == nil {
		return
	}
	_, _ = io.WriteString(stderr, err.Error())
	_, _ = io.WriteString(stderr, "\n")
}

func cloneStringSlice(values []string) []string {
	return append([]string(nil), values...)
}

func currentUTC() time.Time {
	return time.Now().UTC()
}

func hasContentTarget(platform, externalID string) bool {
	return strings.TrimSpace(platform) != "" && strings.TrimSpace(externalID) != ""
}
