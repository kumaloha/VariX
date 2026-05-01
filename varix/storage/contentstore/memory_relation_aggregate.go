package contentstore

import (
	"github.com/kumaloha/VariX/varix/memory"
	"sort"
	"strings"
	"time"
)

func strongerTraceability(left, right memory.TraceabilityStatus) memory.TraceabilityStatus {
	order := map[memory.TraceabilityStatus]int{
		memory.TraceabilityWeak:     0,
		memory.TraceabilityPartial:  1,
		memory.TraceabilityComplete: 2,
	}
	if order[right] > order[left] {
		return right
	}
	if left == "" {
		return right
	}
	return left
}

func ensureAggregateState(states map[string]*aggregateState, key string) *aggregateState {
	if state, ok := states[key]; ok {
		return state
	}
	state := &aggregateState{traceabilityStatus: memory.TraceabilityWeak}
	states[key] = state
	return state
}

type aggregateState struct {
	relationIDs         []string
	neighborEntityIDs   []string
	mechanismLabels     []string
	activeConclusionIDs []string
	traceabilityStatus  memory.TraceabilityStatus
	coverageScore       float64
	conflictCount       int
}

type aggregateSnapshot struct {
	aggregateID         string
	relationIDs         []string
	neighborEntityIDs   []string
	mechanismLabels     []string
	coverageScore       float64
	conflictCount       int
	activeConclusionIDs []string
	traceabilityStatus  memory.TraceabilityStatus
}

func buildDriverAggregatesFromState(states map[string]*aggregateState, now time.Time) []memory.DriverAggregate {
	snapshots := aggregateSnapshots(states)
	out := make([]memory.DriverAggregate, 0, len(snapshots))
	for _, snapshot := range snapshots {
		out = append(out, memory.DriverAggregate{
			AggregateID:         snapshot.aggregateID,
			DriverEntityID:      trimAggregateEntityID(snapshot.aggregateID),
			RelationIDs:         snapshot.relationIDs,
			TargetEntityIDs:     snapshot.neighborEntityIDs,
			MechanismLabels:     snapshot.mechanismLabels,
			CoverageScore:       snapshot.coverageScore,
			ConflictCount:       snapshot.conflictCount,
			ActiveConclusionIDs: snapshot.activeConclusionIDs,
			TraceabilityStatus:  snapshot.traceabilityStatus,
			AsOf:                now,
			CreatedAt:           now,
		})
	}
	return out
}

func buildTargetAggregatesFromState(states map[string]*aggregateState, now time.Time) []memory.TargetAggregate {
	snapshots := aggregateSnapshots(states)
	out := make([]memory.TargetAggregate, 0, len(snapshots))
	for _, snapshot := range snapshots {
		out = append(out, memory.TargetAggregate{
			AggregateID:         snapshot.aggregateID,
			TargetEntityID:      trimAggregateEntityID(snapshot.aggregateID),
			RelationIDs:         snapshot.relationIDs,
			DriverEntityIDs:     snapshot.neighborEntityIDs,
			MechanismLabels:     snapshot.mechanismLabels,
			CoverageScore:       snapshot.coverageScore,
			ConflictCount:       snapshot.conflictCount,
			ActiveConclusionIDs: snapshot.activeConclusionIDs,
			TraceabilityStatus:  snapshot.traceabilityStatus,
			AsOf:                now,
			CreatedAt:           now,
		})
	}
	return out
}

func aggregateSnapshots(states map[string]*aggregateState) []aggregateSnapshot {
	out := make([]aggregateSnapshot, 0, len(states))
	for entityID, state := range states {
		out = append(out, aggregateSnapshot{
			aggregateID:         entityID + "-aggregate",
			relationIDs:         uniquePreservingOrder(state.relationIDs),
			neighborEntityIDs:   uniquePreservingOrder(state.neighborEntityIDs),
			mechanismLabels:     uniquePreservingOrder(state.mechanismLabels),
			coverageScore:       state.coverageScore,
			conflictCount:       state.conflictCount,
			activeConclusionIDs: uniquePreservingOrder(state.activeConclusionIDs),
			traceabilityStatus:  state.traceabilityStatus,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].aggregateID < out[j].aggregateID })
	return out
}

func trimAggregateEntityID(aggregateID string) string {
	return strings.TrimSuffix(aggregateID, "-aggregate")
}

func maxFloat(left, right float64) float64 {
	if right > left {
		return right
	}
	return left
}

func conclusionIDsForThesis(conclusions []memory.CognitiveConclusion) []string {
	out := make([]string, 0, len(conclusions))
	for _, conclusion := range conclusions {
		out = append(out, conclusion.ConclusionID)
	}
	return out
}

func updateAggregateState(state *aggregateState, relationID, neighborEntityID, mechanismLabel string, coverageScore float64, traceabilityStatus memory.TraceabilityStatus, conclusionIDs []string) {
	if state == nil {
		return
	}
	state.relationIDs = append(state.relationIDs, relationID)
	state.neighborEntityIDs = append(state.neighborEntityIDs, neighborEntityID)
	state.mechanismLabels = append(state.mechanismLabels, mechanismLabel)
	state.coverageScore = maxFloat(state.coverageScore, coverageScore)
	state.traceabilityStatus = strongerTraceability(state.traceabilityStatus, traceabilityStatus)
	state.activeConclusionIDs = append(state.activeConclusionIDs, conclusionIDs...)
}
