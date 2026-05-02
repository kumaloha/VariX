package contentstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

func (s *SQLiteStore) runProjectionDirtyMark(ctx context.Context, mark ProjectionDirtyMark, now time.Time, state *projectionDirtyUserState) error {
	userID, err := normalizeRequiredUserID(mark.UserID)
	if err != nil {
		return err
	}
	switch strings.TrimSpace(mark.Layer) {
	case "event":
		_, err = s.RunEventGraphProjection(ctx, userID, now)
		if err == nil && state != nil {
			state.eventRefreshed = true
		}
	case "paradigm":
		_, err = s.RunParadigmProjection(ctx, userID, now)
		if err == nil && state != nil {
			state.paradigmRefreshed = true
		}
	case "global-synthesis":
		refreshProjections := state == nil || !state.eventRefreshed || !state.paradigmRefreshed
		_, err = s.runGlobalMemorySynthesis(ctx, userID, now, refreshProjections)
		if err == nil && refreshProjections && state != nil {
			state.eventRefreshed = true
			state.paradigmRefreshed = true
		}
	case "subject-timeline":
		if strings.TrimSpace(mark.Subject) == "" {
			return fmt.Errorf("subject-timeline mark requires subject")
		}
		return nil
	case "subject-horizon":
		if strings.TrimSpace(mark.Subject) == "" || strings.TrimSpace(mark.Horizon) == "" {
			return fmt.Errorf("subject-horizon mark requires subject and horizon")
		}
		var item memory.SubjectHorizonMemory
		var graphs []model.ContentSubgraph
		hasGraphInputs := false
		if state != nil {
			graphs, err = state.memoryContentGraphsBySubject(ctx, userID, mark.Subject, s.ListMemoryContentGraphsBySubject)
			if err != nil {
				return err
			}
			hasGraphInputs = true
		}
		var resolve projectionSubjectResolver
		if state != nil {
			resolve = func(ctx context.Context, node model.ContentNode) (string, error) {
				return state.canonicalGraphNodeSubject(ctx, node, s.resolveCanonicalGraphNodeSubject)
			}
		}
		item, err = s.getSubjectHorizonMemory(ctx, userID, mark.Subject, mark.Horizon, now, true, graphs, hasGraphInputs, resolve)
		if err == nil && state != nil {
			state.storeSubjectHorizon(item)
		}
	case "subject-experience":
		if strings.TrimSpace(mark.Subject) == "" {
			return fmt.Errorf("subject-experience mark requires subject")
		}
		preloaded := map[string]memory.SubjectHorizonMemory(nil)
		if state != nil {
			preloaded = state.preloadedSubjectHorizons(mark.Subject, defaultSubjectExperienceHorizons)
		}
		_, err = s.getSubjectExperienceMemoryWithHorizonInputs(ctx, userID, mark.Subject, defaultSubjectExperienceHorizons, now, false, preloaded)
	default:
		err = fmt.Errorf("unsupported projection layer %q", mark.Layer)
	}
	return err
}
