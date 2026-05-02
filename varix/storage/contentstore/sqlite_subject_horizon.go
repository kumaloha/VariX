package contentstore

import (
	"context"
	"fmt"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
	"strings"
	"time"
)

type projectionSubjectResolver func(context.Context, model.ContentNode) (string, error)

func (s *SQLiteStore) GetSubjectHorizonMemory(ctx context.Context, userID, subject, horizon string, now time.Time, refresh bool) (memory.SubjectHorizonMemory, error) {
	return s.getSubjectHorizonMemory(ctx, userID, subject, horizon, now, refresh, nil, false, nil)
}

func (s *SQLiteStore) getSubjectHorizonMemory(ctx context.Context, userID, subject, horizon string, now time.Time, refresh bool, graphInputs []model.ContentSubgraph, hasGraphInputs bool, resolveSubject projectionSubjectResolver) (memory.SubjectHorizonMemory, error) {
	userID, err := normalizeRequiredUserID(userID)
	if err != nil {
		return memory.SubjectHorizonMemory{}, err
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return memory.SubjectHorizonMemory{}, fmt.Errorf("subject is required")
	}
	spec, err := subjectHorizonSpecFor(horizon)
	if err != nil {
		return memory.SubjectHorizonMemory{}, err
	}
	now = normalizeNow(now)
	canonicalSubject, err := s.resolveCanonicalListSubject(ctx, subject)
	if err != nil {
		return memory.SubjectHorizonMemory{}, err
	}
	if !refresh {
		cached, ok, err := s.getCachedSubjectHorizonMemory(ctx, userID, canonicalSubject, spec.Horizon, now)
		if err != nil {
			return memory.SubjectHorizonMemory{}, err
		}
		if ok {
			return cached, nil
		}
	}
	if !hasGraphInputs {
		graphInputs, err = s.ListMemoryContentGraphsBySubject(ctx, userID, canonicalSubject)
		if err != nil {
			return memory.SubjectHorizonMemory{}, err
		}
	}
	out, err := s.buildSubjectHorizonMemoryFromGraphs(ctx, userID, subject, canonicalSubject, spec, now, graphInputs, map[string]string{}, resolveSubject)
	if err != nil {
		return memory.SubjectHorizonMemory{}, err
	}
	if err := s.upsertSubjectHorizonMemory(ctx, out); err != nil {
		return memory.SubjectHorizonMemory{}, err
	}
	return out, nil
}
