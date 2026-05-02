package ingest

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/kumaloha/VariX/varix/config"
	"github.com/kumaloha/VariX/varix/ingest/assets"
	"github.com/kumaloha/VariX/varix/ingest/dispatcher"
	"github.com/kumaloha/VariX/varix/ingest/polling"
	"github.com/kumaloha/VariX/varix/ingest/provenance"
	"github.com/kumaloha/VariX/varix/ingest/router"
	"github.com/kumaloha/VariX/varix/ingest/sources/bilibili"
	"github.com/kumaloha/VariX/varix/ingest/sources/rss"
	"github.com/kumaloha/VariX/varix/ingest/sources/search"
	"github.com/kumaloha/VariX/varix/ingest/sources/twitter"
	"github.com/kumaloha/VariX/varix/ingest/sources/web"
	"github.com/kumaloha/VariX/varix/ingest/sources/weibo"
	"github.com/kumaloha/VariX/varix/ingest/sources/youtube"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

type Runtime struct {
	Settings   config.Settings
	Store      *contentstore.SQLiteStore
	Polling    *polling.Service
	Provenance *provenance.Service
	Dispatcher *dispatcher.Service
}

func NewRuntime(projectRoot string) (*Runtime, error) {
	settings := config.DefaultSettings(projectRoot)
	switch settings.StoreBackend {
	case "sqlite", "":
	case "json":
		return nil, fmt.Errorf("json store backend has been removed; use sqlite")
	default:
		return nil, fmt.Errorf("unsupported store backend: %s", settings.StoreBackend)
	}
	store, err := contentstore.NewSQLiteStore(settings.ContentDBPath)
	if err != nil {
		return nil, err
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Timeout: 30 * time.Second, Jar: jar}
	resolver := provenance.NewHTTPResolver(httpClient)

	webSource := web.New(httpClient)
	rssSource := rss.New(httpClient)
	weiboSource := weibo.NewDefault(projectRoot, httpClient)
	twitterSource := twitter.NewDefault(projectRoot, httpClient)
	youtubeSource := youtube.NewDefault(projectRoot, httpClient)
	bilibiliSource := bilibili.NewDefault(projectRoot)

	dispatch := dispatcher.New(
		router.Parse,
		[]dispatcher.ItemSource{
			webSource,
			youtubeSource,
			bilibiliSource,
			twitterSource,
			weiboSource,
		},
		[]dispatcher.Discoverer{
			rssSource,
			weiboSource,
			search.NewGoogle(types.PlatformTwitter, "x.com", httpClient),
			search.NewGoogle(types.PlatformWeibo, "weibo.com", httpClient),
			search.NewGoogle(types.PlatformYouTube, "youtube.com", httpClient),
			search.NewGoogle(types.PlatformBilibili, "bilibili.com", httpClient),
			search.NewGoogle(types.PlatformWeb, "", httpClient),
		},
		resolver,
	)

	return &Runtime{
		Settings: settings,
		Store:    store,
		Polling: polling.New(
			store,
			dispatch,
			provenance.Enricher{},
			polling.WithStoredCaptureReuse(settings.ReuseStoredTranscripts),
			polling.WithAttachmentLocalizer(assets.New(settings.AssetsDir, httpClient)),
		),
		Provenance: provenance.NewService(store, provenance.NewRuleFinderWithResolver(resolver), provenance.DeterministicJudge{}),
		Dispatcher: dispatch,
	}, nil
}
