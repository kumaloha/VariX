package contentstore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

func (s *SQLiteStore) RunProjectionDirtySweep(ctx context.Context, userID string, limit int, now time.Time) (ProjectionDirtySweepResult, error) {
	return s.RunProjectionDirtySweepWithWorkers(ctx, userID, limit, 1, now)
}

func (s *SQLiteStore) RunProjectionDirtySweepWithWorkers(ctx context.Context, userID string, limit int, workers int, now time.Time) (ProjectionDirtySweepResult, error) {
	userID = strings.TrimSpace(userID)
	if limit <= 0 {
		limit = 100
	}
	if workers <= 0 {
		workers = 1
	}
	now = normalizeRecordedTime(now)
	result := ProjectionDirtySweepResult{
		UserID:  userID,
		Limit:   limit,
		Workers: workers,
	}
	marks, err := s.ListProjectionDirtyMarks(ctx, userID, limit)
	if err != nil {
		return result, err
	}
	result = mergeProjectionDirtySweepResult(result, runProjectionDirtyMarkGroups(ctx, marks, workers, func(ctx context.Context, mark ProjectionDirtyMark, state *projectionDirtyUserState) error {
		return s.runProjectionDirtyMark(ctx, mark, now, state)
	}, s.ClearProjectionDirtyMarks))
	remaining, err := s.countProjectionDirtyMarks(ctx, userID)
	if err != nil {
		return result, err
	}
	result.Remaining = remaining
	if result.Completed == 0 {
		result.Layers = nil
	}
	if result.Failed > 0 {
		return result, fmt.Errorf("projection dirty sweep failed for %d mark(s)", result.Failed)
	}
	return result, nil
}

func runProjectionDirtyMarkGroups(ctx context.Context, marks []ProjectionDirtyMark, workers int, runner projectionDirtyMarkRunner, clearer projectionDirtyMarkClearer) ProjectionDirtySweepResult {
	if workers <= 0 {
		workers = 1
	}
	result := ProjectionDirtySweepResult{Workers: workers, Scanned: len(marks), Layers: map[string]int{}}
	groups := groupProjectionDirtyMarksByUser(marks)
	if len(groups) == 0 {
		result.Layers = nil
		return result
	}
	if workers > len(groups) {
		workers = len(groups)
	}
	jobs := make(chan []ProjectionDirtyMark)
	results := make(chan ProjectionDirtySweepResult, len(groups))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for group := range jobs {
				results <- runProjectionDirtyMarkGroup(ctx, group, runner, clearer)
			}
		}()
	}
	for _, group := range groups {
		jobs <- group
	}
	close(jobs)
	wg.Wait()
	close(results)
	for item := range results {
		result = mergeProjectionDirtySweepResult(result, item)
	}
	if result.Completed == 0 {
		result.Layers = nil
	}
	return result
}

func groupProjectionDirtyMarksByUser(marks []ProjectionDirtyMark) [][]ProjectionDirtyMark {
	order := make([]string, 0)
	byUser := map[string][]ProjectionDirtyMark{}
	for _, mark := range marks {
		userID := strings.TrimSpace(mark.UserID)
		if _, ok := byUser[userID]; !ok {
			order = append(order, userID)
		}
		byUser[userID] = append(byUser[userID], mark)
	}
	out := make([][]ProjectionDirtyMark, 0, len(order))
	for _, userID := range order {
		out = append(out, byUser[userID])
	}
	return out
}

func runProjectionDirtyMarkGroup(ctx context.Context, marks []ProjectionDirtyMark, runner projectionDirtyMarkRunner, clearer projectionDirtyMarkClearer) ProjectionDirtySweepResult {
	result := ProjectionDirtySweepResult{Layers: map[string]int{}}
	state := &projectionDirtyUserState{}
	successful := make([]ProjectionDirtyMark, 0, len(marks))
	for _, phase := range projectionDirtyMarkGroupPhases(orderProjectionDirtyMarksForSweep(marks)) {
		if phase.concurrent {
			outcomes := runProjectionDirtyMarksConcurrently(ctx, phase.marks, projectionDirtySubjectWorkers, runner, state)
			for _, outcome := range outcomes {
				recordProjectionDirtyRunOutcome(&result, &successful, outcome)
			}
			continue
		}
		for _, mark := range phase.marks {
			recordProjectionDirtyRunOutcome(&result, &successful, runProjectionDirtyMarkWithState(ctx, mark, runner, state))
		}
	}
	if len(successful) > 0 {
		if err := clearer(ctx, successful); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, projectionDirtySweepError(successful[0], err))
			successful = nil
		}
	}
	for _, mark := range successful {
		result.Completed++
		result.Layers[strings.TrimSpace(mark.Layer)]++
	}
	if result.Completed == 0 {
		result.Layers = nil
	}
	return result
}

type projectionDirtyMarkPhase struct {
	marks      []ProjectionDirtyMark
	concurrent bool
}

type projectionDirtyRunOutcome struct {
	mark ProjectionDirtyMark
	err  error
}

func projectionDirtyMarkGroupPhases(marks []ProjectionDirtyMark) []projectionDirtyMarkPhase {
	var base []ProjectionDirtyMark
	var global []ProjectionDirtyMark
	var horizons []ProjectionDirtyMark
	var experience []ProjectionDirtyMark
	var other []ProjectionDirtyMark
	for _, mark := range marks {
		switch strings.TrimSpace(mark.Layer) {
		case "event", "paradigm":
			base = append(base, mark)
		case "global-v2":
			global = append(global, mark)
		case "subject-horizon":
			horizons = append(horizons, mark)
		case "subject-experience":
			experience = append(experience, mark)
		default:
			other = append(other, mark)
		}
	}
	phases := make([]projectionDirtyMarkPhase, 0, 6)
	appendPhase := func(items []ProjectionDirtyMark, concurrent bool) {
		if len(items) > 0 {
			phases = append(phases, projectionDirtyMarkPhase{marks: items, concurrent: concurrent})
		}
	}
	appendPhase(base, false)
	appendPhase(global, false)
	appendPhase(horizons, true)
	appendPhase(experience, false)
	appendPhase(other, false)
	return phases
}

func runProjectionDirtyMarksConcurrently(ctx context.Context, marks []ProjectionDirtyMark, workers int, runner projectionDirtyMarkRunner, state *projectionDirtyUserState) []projectionDirtyRunOutcome {
	if len(marks) == 0 {
		return nil
	}
	if workers <= 0 || workers > len(marks) {
		workers = len(marks)
	}
	jobs := make(chan ProjectionDirtyMark)
	outcomes := make(chan projectionDirtyRunOutcome, len(marks))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for mark := range jobs {
				outcomes <- runProjectionDirtyMarkWithState(ctx, mark, runner, state)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, mark := range marks {
			jobs <- mark
		}
	}()
	wg.Wait()
	close(outcomes)
	out := make([]projectionDirtyRunOutcome, 0, len(marks))
	for outcome := range outcomes {
		out = append(out, outcome)
	}
	return out
}

func runProjectionDirtyMarkWithState(ctx context.Context, mark ProjectionDirtyMark, runner projectionDirtyMarkRunner, state *projectionDirtyUserState) projectionDirtyRunOutcome {
	if err := ctx.Err(); err != nil {
		return projectionDirtyRunOutcome{mark: mark, err: err}
	}
	return projectionDirtyRunOutcome{mark: mark, err: runner(ctx, mark, state)}
}

func recordProjectionDirtyRunOutcome(result *ProjectionDirtySweepResult, successful *[]ProjectionDirtyMark, outcome projectionDirtyRunOutcome) {
	if outcome.err != nil {
		result.Failed++
		result.Errors = append(result.Errors, projectionDirtySweepError(outcome.mark, outcome.err))
		return
	}
	*successful = append(*successful, outcome.mark)
}

func mergeProjectionDirtySweepResult(base ProjectionDirtySweepResult, add ProjectionDirtySweepResult) ProjectionDirtySweepResult {
	base.Scanned += add.Scanned
	base.Completed += add.Completed
	base.Failed += add.Failed
	if len(add.Errors) > 0 {
		base.Errors = append(base.Errors, add.Errors...)
	}
	if len(add.Layers) > 0 {
		if base.Layers == nil {
			base.Layers = map[string]int{}
		}
		for layer, count := range add.Layers {
			base.Layers[layer] += count
		}
	}
	return base
}

func orderProjectionDirtyMarksForSweep(marks []ProjectionDirtyMark) []ProjectionDirtyMark {
	out := append([]ProjectionDirtyMark(nil), marks...)
	sort.SliceStable(out, func(i, j int) bool {
		return projectionDirtyLayerPriority(out[i].Layer) < projectionDirtyLayerPriority(out[j].Layer)
	})
	return out
}

func projectionDirtyLayerPriority(layer string) int {
	switch strings.TrimSpace(layer) {
	case "event", "paradigm":
		return 0
	case "global-v2":
		return 1
	default:
		return 2
	}
}

func projectionDirtySweepError(mark ProjectionDirtyMark, err error) ProjectionDirtySweepError {
	return ProjectionDirtySweepError{
		DirtyID: mark.ID,
		Layer:   strings.TrimSpace(mark.Layer),
		Subject: strings.TrimSpace(mark.Subject),
		Horizon: strings.TrimSpace(mark.Horizon),
		Error:   err.Error(),
	}
}
