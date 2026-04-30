package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

type Store interface {
	BuildSubjectTimeline(ctx context.Context, userID, subject string, now time.Time) (memory.SubjectTimeline, error)
	GetSubjectHorizonMemory(ctx context.Context, userID, subject, horizon string, now time.Time, refresh bool) (memory.SubjectHorizonMemory, error)
	GetSubjectExperienceMemory(ctx context.Context, userID, subject string, horizons []string, now time.Time, refresh bool) (memory.SubjectExperienceMemory, error)
	HasProjectionDirtyMark(ctx context.Context, userID, layer, subject, horizon string) (bool, error)
}

type Server struct {
	store Store
	now   func() time.Time
}

type Freshness struct {
	GeneratedAt   string `json:"generated_at,omitempty"`
	InputHash     string `json:"input_hash,omitempty"`
	Stale         bool   `json:"stale"`
	Pending       bool   `json:"pending"`
	NextRefreshAt string `json:"next_refresh_at,omitempty"`
}

func NewServer(store Store) *Server {
	return &Server{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/memory/subjects/", s.handleSubjectMemory)
	return withJSONHeaders(mux)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSubjectMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	subject, view, ok := parseSubjectMemoryPath(r.URL)
	if !ok {
		writeAPIError(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "subject memory endpoint not found")
		return
	}
	userID := requestUserID(r)
	if userID == "" {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_ERROR", "user is required")
		return
	}
	switch view {
	case "timeline":
		s.handleSubjectTimeline(w, r, userID, subject)
	case "horizons":
		s.handleSubjectHorizons(w, r, userID, subject)
	case "experience":
		s.handleSubjectExperience(w, r, userID, subject)
	default:
		writeAPIError(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "subject memory endpoint not found")
	}
}

func (s *Server) handleSubjectTimeline(w http.ResponseWriter, r *http.Request, userID, subject string) {
	timeline, err := s.store.BuildSubjectTimeline(r.Context(), userID, subject, s.now())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"timeline":  timeline,
		"freshness": Freshness{GeneratedAt: timeline.GeneratedAt.Format(time.RFC3339)},
	})
}

func (s *Server) handleSubjectHorizons(w http.ResponseWriter, r *http.Request, userID, subject string) {
	horizons := splitHorizons(r.URL.Query().Get("horizons"))
	items := make([]memory.SubjectHorizonMemory, 0, len(horizons))
	for _, horizon := range horizons {
		item, err := s.store.GetSubjectHorizonMemory(r.Context(), userID, subject, horizon, s.now(), false)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		items = append(items, item)
	}
	fresh := Freshness{}
	if len(items) > 0 {
		fresh = s.freshness(r.Context(), userID, "subject-horizon", items[0].CanonicalSubject, "", items[0].GeneratedAt, items[0].NextRefreshAt)
		fresh.InputHash = combinedInputHash(items)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"freshness": fresh,
	})
}

func (s *Server) handleSubjectExperience(w http.ResponseWriter, r *http.Request, userID, subject string) {
	horizons := splitHorizons(r.URL.Query().Get("horizons"))
	out, err := s.store.GetSubjectExperienceMemory(r.Context(), userID, subject, horizons, s.now(), false)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"experience": out,
		"freshness":  s.freshness(r.Context(), userID, "subject-experience", out.CanonicalSubject, "", out.GeneratedAt, ""),
	})
}

func (s *Server) freshness(ctx context.Context, userID, layer, subject, horizon, generatedAt, nextRefreshAt string) Freshness {
	fresh := Freshness{GeneratedAt: generatedAt, NextRefreshAt: nextRefreshAt}
	pending, err := s.store.HasProjectionDirtyMark(ctx, userID, layer, subject, horizon)
	if err != nil {
		return fresh
	}
	if pending {
		fresh.Stale = true
		fresh.Pending = true
	}
	return fresh
}

func parseSubjectMemoryPath(u *url.URL) (subject string, view string, ok bool) {
	const prefix = "/memory/subjects/"
	rest := strings.TrimPrefix(u.EscapedPath(), prefix)
	if rest == u.EscapedPath() {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	subject, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(subject) == "" {
		return "", "", false
	}
	view = strings.TrimSpace(parts[1])
	return strings.TrimSpace(subject), view, view != ""
}

func requestUserID(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-User-ID")); value != "" {
		return value
	}
	return strings.TrimSpace(r.URL.Query().Get("user"))
}

func splitHorizons(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{"1w", "1m", "1q", "1y", "2y", "5y"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return []string{"1w", "1m", "1q", "1y", "2y", "5y"}
	}
	return out
}

func combinedInputHash(items []memory.SubjectHorizonMemory) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if item.InputHash != "" {
			parts = append(parts, item.Horizon+":"+item.InputHash)
		}
	}
	return strings.Join(parts, "|")
}

func withJSONHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

func writeStoreError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, context.Canceled) {
		writeAPIError(w, http.StatusRequestTimeout, "REQUEST_CANCELLED", err.Error())
		return
	}
	writeAPIError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"code":    code,
		"message": message,
	})
}
