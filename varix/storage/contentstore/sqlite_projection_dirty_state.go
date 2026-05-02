package contentstore

import (
	"context"
	"strings"
	"sync"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

type projectionDirtyUserState struct {
	mu                sync.Mutex
	eventRefreshed    bool
	paradigmRefreshed bool
	subjectHorizons   map[string]memory.SubjectHorizonMemory
	subjectGraphs     map[string][]model.ContentSubgraph
	subjectGraphLoads map[string]*projectionDirtyContentGraphLoad
	canonicalSubjects map[string]string
}

type projectionDirtyContentGraphLoad struct {
	done   chan struct{}
	graphs []model.ContentSubgraph
	err    error
}

func (state *projectionDirtyUserState) memoryContentGraphsBySubject(ctx context.Context, userID, subject string, load func(context.Context, string, string) ([]model.ContentSubgraph, error)) ([]model.ContentSubgraph, error) {
	key := strings.TrimSpace(userID) + "\x00" + normalizeDirtyDimension(subject)
	return state.memoryContentGraphsForKey(ctx, key, func(ctx context.Context) ([]model.ContentSubgraph, error) {
		return load(ctx, userID, subject)
	})
}

func (state *projectionDirtyUserState) memoryContentGraphsForKey(ctx context.Context, key string, load func(context.Context) ([]model.ContentSubgraph, error)) ([]model.ContentSubgraph, error) {
	if state == nil {
		return load(ctx)
	}
	state.mu.Lock()
	if state.subjectGraphs != nil {
		if graphs, ok := state.subjectGraphs[key]; ok {
			state.mu.Unlock()
			return graphs, nil
		}
	}
	if state.subjectGraphLoads == nil {
		state.subjectGraphLoads = map[string]*projectionDirtyContentGraphLoad{}
	}
	if inFlight := state.subjectGraphLoads[key]; inFlight != nil {
		done := inFlight.done
		state.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		state.mu.Lock()
		graphs, err := inFlight.graphs, inFlight.err
		state.mu.Unlock()
		return graphs, err
	}
	inFlight := &projectionDirtyContentGraphLoad{done: make(chan struct{})}
	state.subjectGraphLoads[key] = inFlight
	state.mu.Unlock()

	graphs, err := load(ctx)

	state.mu.Lock()
	inFlight.graphs = graphs
	inFlight.err = err
	if err == nil {
		if state.subjectGraphs == nil {
			state.subjectGraphs = map[string][]model.ContentSubgraph{}
		}
		state.subjectGraphs[key] = graphs
	}
	delete(state.subjectGraphLoads, key)
	close(inFlight.done)
	state.mu.Unlock()
	return graphs, err
}

func (state *projectionDirtyUserState) canonicalGraphNodeSubject(ctx context.Context, node model.ContentNode, resolve func(context.Context, model.ContentNode, map[string]string) (string, error)) (string, error) {
	if state == nil {
		return resolve(ctx, node, nil)
	}
	key := normalizeDirtyDimension(firstTrimmed(node.SubjectCanonical, node.SubjectText))
	if key == "" {
		return "", nil
	}
	state.mu.Lock()
	if state.canonicalSubjects != nil {
		if subject, ok := state.canonicalSubjects[key]; ok {
			state.mu.Unlock()
			return subject, nil
		}
	}
	state.mu.Unlock()

	subject, err := resolve(ctx, node, nil)
	if err != nil {
		return "", err
	}
	subject = strings.TrimSpace(subject)

	state.mu.Lock()
	if state.canonicalSubjects == nil {
		state.canonicalSubjects = map[string]string{}
	}
	state.canonicalSubjects[key] = subject
	state.mu.Unlock()
	return subject, nil
}

func (state *projectionDirtyUserState) storeSubjectHorizon(item memory.SubjectHorizonMemory) {
	if state == nil || strings.TrimSpace(item.Horizon) == "" {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.subjectHorizons == nil {
		state.subjectHorizons = map[string]memory.SubjectHorizonMemory{}
	}
	state.subjectHorizons[subjectHorizonStateKey(item.CanonicalSubject, item.Horizon)] = item
	state.subjectHorizons[subjectHorizonStateKey(item.Subject, item.Horizon)] = item
}

func (state *projectionDirtyUserState) preloadedSubjectHorizons(subject string, horizons []string) map[string]memory.SubjectHorizonMemory {
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.subjectHorizons) == 0 {
		return nil
	}
	out := map[string]memory.SubjectHorizonMemory{}
	for _, horizon := range horizons {
		key := subjectHorizonStateKey(subject, horizon)
		if item, ok := state.subjectHorizons[key]; ok {
			out[strings.TrimSpace(horizon)] = item
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func subjectHorizonStateKey(subject, horizon string) string {
	return normalizeDirtyDimension(subject) + "\x00" + strings.TrimSpace(horizon)
}
