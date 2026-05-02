package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestServerSubjectTimelineFreshnessIgnoresDirtyMarks(t *testing.T) {
	store := &fakeStore{
		timeline: memory.SubjectTimeline{
			UserID:           "u1",
			Subject:          "美股",
			CanonicalSubject: "美股",
			GeneratedAt:      time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		},
		marks: []contentstore.ProjectionDirtyMark{{UserID: "u1", Layer: "subject-timeline", Subject: "美股", Status: contentstore.ProjectionDirtyPending}},
	}
	srv := NewServer(store)
	req := httptest.NewRequest(http.MethodGet, "/memory/subjects/%E7%BE%8E%E8%82%A1/timeline?user=u1", nil)
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
	if payload.Freshness.Stale || payload.Freshness.Pending {
		t.Fatalf("freshness = %#v, want computed timeline without pending projection state", payload.Freshness)
	}
	if payload.Freshness.GeneratedAt != "2026-04-30T12:00:00Z" {
		t.Fatalf("generated_at = %q, want timeline generation time", payload.Freshness.GeneratedAt)
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

func TestBearerTokenAuthRequiresConfiguredToken(t *testing.T) {
	handler := BearerTokenAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "secret-token")

	req := httptest.NewRequest(http.MethodGet, "/memory/subjects/a/timeline?user=u1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status without token = %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/memory/subjects/a/timeline?user=u1", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status with token = %d, want 204", rec.Code)
	}
}

func TestBearerTokenAuthBindsUserID(t *testing.T) {
	srv := NewServer(&fakeStore{})
	handler := BearerTokenAuth(srv.Handler(), "secret-token", "owner")
	req := httptest.NewRequest(http.MethodGet, "/memory/subjects/a/timeline?user=intruder", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("X-User-ID", "also-intruder")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var payload struct {
		Timeline memory.SubjectTimeline `json:"timeline"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error = %v", err)
	}
	if payload.Timeline.UserID != "owner" {
		t.Fatalf("timeline user_id = %q, want bound token user", payload.Timeline.UserID)
	}
}

func TestServerStoreErrorHidesInternalMessage(t *testing.T) {
	srv := NewServer(&fakeStore{storeError: errors.New("sql: leaked /var/lib/varix/content.db")})
	req := httptest.NewRequest(http.MethodGet, "/memory/subjects/a/timeline?user=u1", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s, want 500", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "content.db") || strings.Contains(rec.Body.String(), "sql:") {
		t.Fatalf("body leaks internal error: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "internal server error") {
		t.Fatalf("body = %s, want generic internal server error", rec.Body.String())
	}
}

type fakeStore struct {
	timeline   memory.SubjectTimeline
	horizons   map[string]memory.SubjectHorizonMemory
	experience memory.SubjectExperienceMemory
	marks      []contentstore.ProjectionDirtyMark
	storeError error
}

func (f *fakeStore) BuildSubjectTimeline(_ context.Context, userID, subject string, now time.Time) (memory.SubjectTimeline, error) {
	if f.storeError != nil {
		return memory.SubjectTimeline{}, f.storeError
	}
	if f.timeline.Subject != "" {
		return f.timeline, nil
	}
	return memory.SubjectTimeline{UserID: userID, Subject: subject, CanonicalSubject: subject, GeneratedAt: now}, nil
}

func (f *fakeStore) GetSubjectHorizonMemory(_ context.Context, userID, subject, horizon string, now time.Time, _ bool) (memory.SubjectHorizonMemory, error) {
	if f.storeError != nil {
		return memory.SubjectHorizonMemory{}, f.storeError
	}
	if f.horizons != nil {
		if item, ok := f.horizons[horizon]; ok {
			return item, nil
		}
	}
	return memory.SubjectHorizonMemory{UserID: userID, Subject: subject, CanonicalSubject: subject, Horizon: horizon, GeneratedAt: now.Format(time.RFC3339), InputHash: "hash-" + horizon}, nil
}

func (f *fakeStore) GetSubjectExperienceMemory(_ context.Context, userID, subject string, horizons []string, now time.Time, _ bool) (memory.SubjectExperienceMemory, error) {
	if f.storeError != nil {
		return memory.SubjectExperienceMemory{}, f.storeError
	}
	if f.experience.Subject != "" {
		f.experience.Horizons = horizons
		return f.experience, nil
	}
	return memory.SubjectExperienceMemory{UserID: userID, Subject: subject, CanonicalSubject: subject, Horizons: horizons, GeneratedAt: now.Format(time.RFC3339), InputHash: "hash-experience"}, nil
}

func (f *fakeStore) HasProjectionDirtyMark(_ context.Context, userID, layer, subject, horizon string) (bool, error) {
	if f.storeError != nil {
		return false, f.storeError
	}
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
