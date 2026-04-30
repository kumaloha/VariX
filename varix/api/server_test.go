package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func TestServerSubjectExperienceReturnsFreshnessPending(t *testing.T) {
	store := &fakeStore{
		experience: memory.SubjectExperienceMemory{
			UserID:           "u1",
			Subject:          "美股",
			CanonicalSubject: "美股",
			Horizons:         []string{"1w", "1m"},
			GeneratedAt:      "2026-04-30T00:00:00Z",
			InputHash:        "hash-experience",
		},
		marks: []contentstore.ProjectionDirtyMark{{UserID: "u1", Layer: "subject-experience", Subject: "美股", Status: contentstore.ProjectionDirtyPending}},
	}
	srv := NewServer(store)
	req := httptest.NewRequest(http.MethodGet, "/memory/subjects/%E7%BE%8E%E8%82%A1/experience?user=u1&horizons=1w,1m", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var payload struct {
		Experience memory.SubjectExperienceMemory `json:"experience"`
		Freshness  Freshness                      `json:"freshness"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error = %v", err)
	}
	if payload.Experience.Subject != "美股" || payload.Experience.Horizons[0] != "1w" {
		t.Fatalf("experience = %#v, want subject and requested horizons", payload.Experience)
	}
	if !payload.Freshness.Stale || !payload.Freshness.Pending {
		t.Fatalf("freshness = %#v, want stale pending", payload.Freshness)
	}
}

func TestServerSubjectExperienceFreshnessDoesNotMissDirtyMarkPastListLimit(t *testing.T) {
	marks := make([]contentstore.ProjectionDirtyMark, 0, 501)
	for i := 0; i < 500; i++ {
		marks = append(marks, contentstore.ProjectionDirtyMark{UserID: "u1", Layer: "subject-experience", Subject: "无关主体", Status: contentstore.ProjectionDirtyPending})
	}
	marks = append(marks, contentstore.ProjectionDirtyMark{UserID: "u1", Layer: "subject-experience", Subject: "美股", Status: contentstore.ProjectionDirtyPending})
	store := &fakeStore{
		experience: memory.SubjectExperienceMemory{
			UserID:           "u1",
			Subject:          "美股",
			CanonicalSubject: "美股",
			Horizons:         []string{"1w"},
			GeneratedAt:      "2026-04-30T00:00:00Z",
		},
		marks: marks,
	}
	srv := NewServer(store)
	req := httptest.NewRequest(http.MethodGet, "/memory/subjects/%E7%BE%8E%E8%82%A1/experience?user=u1&horizons=1w", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var payload struct {
		Freshness Freshness `json:"freshness"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error = %v", err)
	}
	if !payload.Freshness.Stale || !payload.Freshness.Pending {
		t.Fatalf("freshness = %#v, want stale pending despite relevant mark after list limit", payload.Freshness)
	}
}

func TestServerSubjectHorizonsRequiresUser(t *testing.T) {
	srv := NewServer(&fakeStore{})
	req := httptest.NewRequest(http.MethodGet, "/memory/subjects/%E7%BE%8E%E8%82%A1/horizons", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
}

type fakeStore struct {
	timeline   memory.SubjectTimeline
	horizons   map[string]memory.SubjectHorizonMemory
	experience memory.SubjectExperienceMemory
	marks      []contentstore.ProjectionDirtyMark
}

func (f *fakeStore) BuildSubjectTimeline(_ context.Context, userID, subject string, now time.Time) (memory.SubjectTimeline, error) {
	if f.timeline.Subject != "" {
		return f.timeline, nil
	}
	return memory.SubjectTimeline{UserID: userID, Subject: subject, CanonicalSubject: subject, GeneratedAt: now}, nil
}

func (f *fakeStore) GetSubjectHorizonMemory(_ context.Context, userID, subject, horizon string, now time.Time, _ bool) (memory.SubjectHorizonMemory, error) {
	if f.horizons != nil {
		if item, ok := f.horizons[horizon]; ok {
			return item, nil
		}
	}
	return memory.SubjectHorizonMemory{UserID: userID, Subject: subject, CanonicalSubject: subject, Horizon: horizon, GeneratedAt: now.Format(time.RFC3339), InputHash: "hash-" + horizon}, nil
}

func (f *fakeStore) GetSubjectExperienceMemory(_ context.Context, userID, subject string, horizons []string, now time.Time, _ bool) (memory.SubjectExperienceMemory, error) {
	if f.experience.Subject != "" {
		f.experience.Horizons = horizons
		return f.experience, nil
	}
	return memory.SubjectExperienceMemory{UserID: userID, Subject: subject, CanonicalSubject: subject, Horizons: horizons, GeneratedAt: now.Format(time.RFC3339), InputHash: "hash-experience"}, nil
}

func (f *fakeStore) HasProjectionDirtyMark(_ context.Context, userID, layer, subject, horizon string) (bool, error) {
	for _, mark := range f.marks {
		if userID != "" && mark.UserID != userID {
			continue
		}
		if layer != "" && mark.Layer != layer {
			continue
		}
		if subject != "" && mark.Subject != subject {
			continue
		}
		if horizon != "" && mark.Horizon != horizon {
			continue
		}
		return true, nil
	}
	return false, nil
}

func (f *fakeStore) ListProjectionDirtyMarks(_ context.Context, userID string, limit int) ([]contentstore.ProjectionDirtyMark, error) {
	out := make([]contentstore.ProjectionDirtyMark, 0)
	for _, mark := range f.marks {
		if userID == "" || mark.UserID == userID {
			out = append(out, mark)
			if limit > 0 && len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}
