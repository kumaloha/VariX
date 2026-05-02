package main

import (
	"bytes"
	"context"
	"github.com/kumaloha/VariX/varix/ingest"
	"github.com/kumaloha/VariX/varix/model"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"strings"
	"testing"
	"time"
)

func TestRunMemorySubjectTimelineRendersCard(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	timelineNow := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		for _, sg := range []model.ContentSubgraph{
			{
				ID:               "timeline-cg-1",
				ArticleID:        "timeline-cg-1",
				SourcePlatform:   "twitter",
				SourceExternalID: "timeline-cg-1",
				CompileVersion:   model.CompileBridgeVersion,
				CompiledAt:       timelineNow.Format(time.RFC3339),
				UpdatedAt:        timelineNow.Format(time.RFC3339),
				Nodes: []model.ContentNode{{
					ID:                 "n1",
					SourceArticleID:    "timeline-cg-1",
					SourcePlatform:     "twitter",
					SourceExternalID:   "timeline-cg-1",
					RawText:            "美股承压",
					SubjectText:        "美股",
					ChangeText:         "承压",
					TimeStart:          timelineNow.Add(-24 * time.Hour).Format(time.RFC3339),
					Kind:               model.NodeKindObservation,
					GraphRole:          model.GraphRoleTarget,
					IsPrimary:          true,
					VerificationStatus: model.VerificationPending,
				}},
			},
			{
				ID:               "timeline-cg-2",
				ArticleID:        "timeline-cg-2",
				SourcePlatform:   "twitter",
				SourceExternalID: "timeline-cg-2",
				CompileVersion:   model.CompileBridgeVersion,
				CompiledAt:       timelineNow.Format(time.RFC3339),
				UpdatedAt:        timelineNow.Format(time.RFC3339),
				Nodes: []model.ContentNode{{
					ID:                 "n2",
					SourceArticleID:    "timeline-cg-2",
					SourcePlatform:     "twitter",
					SourceExternalID:   "timeline-cg-2",
					RawText:            "美股反弹",
					SubjectText:        "美股",
					ChangeText:         "反弹",
					Kind:               model.NodeKindObservation,
					GraphRole:          model.GraphRoleTarget,
					IsPrimary:          true,
					VerificationStatus: model.VerificationProved,
					VerificationReason: "observed rebound",
					VerificationAsOf:   timelineNow.Format(time.RFC3339),
				}},
			},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-subject-timeline", sg, timelineNow); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "subject-timeline", "--card", "--user", "u-subject-timeline", "--subject", "美股"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("subject-timeline --card code = %d, stderr = %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"Subject Timeline", "美股", "承压", "反弹", timelineNow.Format(time.RFC3339), "twitter:timeline-cg-2#n2", "proved (observed rebound)", "contradicts"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout = %q, want substring %q", got, want)
		}
	}
}

func TestRunMemorySubjectHorizonRendersCard(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	now := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := model.ContentSubgraph{
			ID:               "horizon-card",
			ArticleID:        "horizon-card",
			SourcePlatform:   "twitter",
			SourceExternalID: "horizon-card",
			CompileVersion:   model.CompileBridgeVersion,
			CompiledAt:       now.Format(time.RFC3339),
			UpdatedAt:        now.Format(time.RFC3339),
			Nodes: []model.ContentNode{
				{ID: "d1", SourceArticleID: "horizon-card", SourcePlatform: "twitter", SourceExternalID: "horizon-card", RawText: "油价上涨", SubjectText: "油价", ChangeText: "上涨", TimeStart: now.Format(time.RFC3339), Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationProved},
				{ID: "t1", SourceArticleID: "horizon-card", SourcePlatform: "twitter", SourceExternalID: "horizon-card", RawText: "美股回落", SubjectText: "美股", ChangeText: "回落", TimeStart: now.Format(time.RFC3339), Kind: model.NodeKindObservation, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved},
			},
		}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-subject-horizon", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "subject-horizon", "--card", "--refresh", "--user", "u-subject-horizon", "--subject", "美股", "--horizon", "1w"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("subject-horizon --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Subject Horizon", "Horizon: 1w", "Policy: daily", "美股", "回落", "油价"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestRunMemorySubjectExperienceRendersCard(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	now := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := model.ContentSubgraph{
			ID:               "experience-card",
			ArticleID:        "experience-card",
			SourcePlatform:   "twitter",
			SourceExternalID: "experience-card",
			CompileVersion:   model.CompileBridgeVersion,
			CompiledAt:       now.Format(time.RFC3339),
			UpdatedAt:        now.Format(time.RFC3339),
			Nodes: []model.ContentNode{
				{ID: "d1", SourceArticleID: "experience-card", SourcePlatform: "twitter", SourceExternalID: "experience-card", RawText: "油价上涨", SubjectText: "油价", ChangeText: "上涨", TimeStart: now.Format(time.RFC3339), Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationProved},
				{ID: "t1", SourceArticleID: "experience-card", SourcePlatform: "twitter", SourceExternalID: "experience-card", RawText: "美股回落", SubjectText: "美股", ChangeText: "回落", TimeStart: now.Format(time.RFC3339), Kind: model.NodeKindObservation, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved},
			},
		}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-subject-experience", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "subject-experience", "--card", "--refresh", "--user", "u-subject-experience", "--subject", "美股", "--horizons", "1w,1m"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("subject-experience --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"主体归因总结", "观察窗口: 最近 1w, 最近 1m", "变化数", "因素数", "归因总结", "主要因素", "变化归因", "因素关系", "油价"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
	for _, notWant := range []string{"branch driver", "driver-pattern", "Drivers:", "Key factors:", "Mechanism:", "Transfer:", "Horizons:"} {
		if strings.Contains(stdout.String(), notWant) {
			t.Fatalf("stdout = %q, should not expose internal term %q", stdout.String(), notWant)
		}
	}
	for _, notWant := range []string{"使用方式", "时间尺度含义", "支撑变化"} {
		if strings.Contains(stdout.String(), notWant) {
			t.Fatalf("stdout = %q, should avoid verbose phrase %q", stdout.String(), notWant)
		}
	}
	for _, notWant := range []string{"中间机制未展开", "下次先找"} {
		if strings.Contains(stdout.String(), notWant) {
			t.Fatalf("stdout = %q, should summarize attribution instead of warnings like %q", stdout.String(), notWant)
		}
	}
	if strings.Contains(stdout.String(), "暂不判断因果先后") {
		t.Fatalf("stdout = %q, should avoid awkward relation caveat", stdout.String())
	}
	for _, notWant := range []string{"长期", "中长期", "短期", "时间尺度提示"} {
		if strings.Contains(stdout.String(), notWant) {
			t.Fatalf("stdout = %q, should use recent-window phrasing instead of %q", stdout.String(), notWant)
		}
	}
}
