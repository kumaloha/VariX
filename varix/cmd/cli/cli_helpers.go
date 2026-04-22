package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"strings"

	"github.com/kumaloha/VariX/varix/bootstrap"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func setRawURLFromArg(fs *flag.FlagSet, rawURL *string) {
	if rawURL == nil || strings.TrimSpace(*rawURL) != "" || fs == nil || fs.NArg() == 0 {
		return
	}
	*rawURL = fs.Arg(0)
}

func resolveContentTarget(ctx context.Context, app *bootstrap.App, rawURL, platform, externalID string) (string, string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return strings.TrimSpace(platform), strings.TrimSpace(externalID), nil
	}
	parsed, err := app.Dispatcher.ParseURL(ctx, rawURL)
	if err != nil {
		return "", "", err
	}
	return string(parsed.Platform), parsed.PlatformID, nil
}

func openAppStore(projectRoot string) (*bootstrap.App, *contentstore.SQLiteStore, error) {
	app, err := buildApp(projectRoot)
	if err != nil {
		return nil, nil, err
	}
	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		return nil, nil, err
	}
	return app, store, nil
}

func openStore(projectRoot string) (*contentstore.SQLiteStore, error) {
	_, store, err := openAppStore(projectRoot)
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
