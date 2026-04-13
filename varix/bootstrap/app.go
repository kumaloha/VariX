package bootstrap

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/config"
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
	"github.com/kumaloha/forge/llm"
)

type App struct {
	Settings   config.Settings
	Polling    *polling.Service
	Provenance *provenance.Service
	Dispatcher *dispatcher.Service
}

func BuildApp(projectRoot string) (*App, error) {
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

	provenanceJudge, err := buildProvenanceJudge(projectRoot, settings)
	if err != nil {
		return nil, err
	}

	return &App{
		Settings:   settings,
		Polling:    polling.New(store, dispatch, provenance.Enricher{}),
		Provenance: provenance.NewService(store, provenance.NewRuleFinderWithResolver(resolver), provenanceJudge),
		Dispatcher: dispatch,
	}, nil
}

func buildProvenanceJudge(projectRoot string, settings config.Settings) (provenance.Judge, error) {
	switch strings.ToLower(strings.TrimSpace(settings.ProvenanceJudge)) {
	case "", "deterministic":
		return provenance.DeterministicJudge{}, nil
	case "llm":
		llmCfg, err := config.LoadLLMConfig(projectRoot)
		if err != nil {
			return nil, err
		}
		provider, err := newLLMProvider(projectRoot, llmCfg)
		if err != nil {
			return nil, err
		}
		rt := llm.NewRuntime(llm.RuntimeConfig{
			Provider:  provider,
			LLMConfig: llmCfg,
		})
		return provenance.NewLLMJudge(rt, config.NewPromptLoader(projectRoot))
	default:
		return nil, fmt.Errorf("unsupported provenance judge: %s", settings.ProvenanceJudge)
	}
}

func newLLMProvider(projectRoot string, cfg llm.LLMConfig) (llm.Provider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "dashscope":
		apiKey, ok := config.Get(projectRoot, "DASHSCOPE_API_KEY")
		if !ok || strings.TrimSpace(apiKey) == "" {
			return nil, fmt.Errorf("llm provenance judge requires DASHSCOPE_API_KEY")
		}
		return llm.NewDashscope(
			llm.WithAPIKey(strings.TrimSpace(apiKey)),
			llm.WithAPIBase(strings.TrimSpace(cfg.APIBase)),
		)
	default:
		return nil, fmt.Errorf("unsupported llm provider for provenance judge: %s", cfg.Provider)
	}
}
