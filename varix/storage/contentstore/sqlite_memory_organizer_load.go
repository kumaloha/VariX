package contentstore

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/memory"
)

func loadOrganizationJobSourceData(ctx context.Context, tx *sql.Tx, job memory.OrganizationJob) (organizationJobSourceData, error) {
	nodes, err := listUserMemoryBySourceTx(ctx, tx, job.UserID, job.SourcePlatform, job.SourceExternalID)
	if err != nil {
		return organizationJobSourceData{}, err
	}
	posteriorByMemoryID, err := loadPosteriorStatesBySourceTx(ctx, tx, job.UserID, job.SourcePlatform, job.SourceExternalID)
	if err != nil {
		return organizationJobSourceData{}, err
	}
	record, err := getCompiledOutputTx(ctx, tx, job.SourcePlatform, job.SourceExternalID)
	if err != nil {
		return organizationJobSourceData{}, err
	}
	verifyRecord, err := getVerificationResultTx(ctx, tx, job.SourcePlatform, job.SourceExternalID)
	if err != nil && err != sql.ErrNoRows {
		return organizationJobSourceData{}, err
	}
	verification := effectiveVerification(record, verifyRecord)
	var graphFirstSubgraph graphmodel.ContentSubgraph
	hasGraphFirstSubgraph := false
	if graphFirst, err := getMemoryContentGraphBySourceTx(ctx, tx, job.UserID, job.SourcePlatform, job.SourceExternalID); err == nil {
		graphFirstSubgraph = graphFirst
		hasGraphFirstSubgraph = true
		verification = overlayVerificationFromContentGraph(verification, graphFirst)
	}
	data := organizationJobSourceData{
		record:                        record,
		verification:                  verification,
		nodes:                         nodes,
		posteriorByMemoryID:           posteriorByMemoryID,
		graphFirstSubgraph:            graphFirstSubgraph,
		hasGraphFirstSubgraph:         hasGraphFirstSubgraph,
		graphNodesByID:                map[string]compile.GraphNode{},
		graphFirstNodesByID:           map[string]graphmodel.GraphNode{},
		factStatusByNode:              factStatusMap(verification),
		explicitConditionStatusByNode: explicitConditionStatusMap(verification),
		predictionStatusByNode:        predictionStatusMap(verification),
	}
	for _, node := range record.Output.Graph.Nodes {
		data.graphNodesByID[node.ID] = node
	}
	if hasGraphFirstSubgraph {
		for _, node := range graphFirstSubgraph.Nodes {
			data.graphFirstNodesByID[node.ID] = node
		}
	}
	return data, nil
}

func deriveOrganizationNodeSets(data organizationJobSourceData, now time.Time) organizationNodeSets {
	sets := organizationNodeSets{
		derived:  make([]memory.AcceptedNode, 0, len(data.nodes)),
		active:   make([]memory.AcceptedNode, 0, len(data.nodes)),
		inactive: make([]memory.AcceptedNode, 0, len(data.nodes)),
	}
	for _, node := range data.nodes {
		node = applyPosteriorToAcceptedNode(node, data.posteriorByMemoryID)
		node, graphFirstApplied := applyGraphFirstNodeProjection(node, data.graphFirstNodesByID)
		node = applyCompileNodeProjection(node, data.graphNodesByID, graphFirstApplied)
		sets.derived = append(sets.derived, node)
		if isAcceptedNodeActiveAt(node, now) {
			sets.active = append(sets.active, node)
		} else {
			sets.inactive = append(sets.inactive, node)
		}
	}
	return sets
}

func applyPosteriorToAcceptedNode(node memory.AcceptedNode, posteriorByMemoryID map[int64]posteriorStateRow) memory.AcceptedNode {
	posterior, ok := posteriorByMemoryID[node.MemoryID]
	if !ok {
		return node
	}
	node.PosteriorState = posterior.State
	node.PosteriorDiagnosis = posterior.Diagnosis
	node.PosteriorReason = posterior.Reason
	node.BlockedByNodeIDs = cloneStringSlice(posterior.BlockedByNodeIDs)
	node.PosteriorUpdatedAt = posterior.UpdatedAt
	return node
}

func applyGraphFirstNodeProjection(node memory.AcceptedNode, graphFirstNodesByID map[string]graphmodel.GraphNode) (memory.AcceptedNode, bool) {
	graphFirst, ok := graphFirstNodesByID[node.NodeID]
	if !ok {
		return node, false
	}
	graphFirstApplied := false
	if rawText := strings.TrimSpace(graphFirst.RawText); rawText != "" {
		node.NodeText = rawText
		graphFirstApplied = true
	}
	if graphFirst.Kind == graphmodel.NodeKindPrediction {
		node.NodeKind = string(compile.NodePrediction)
	}
	graphFirstStart, graphFirstEnd := graphFirstValidityWindow(graphFirst)
	if !graphFirstStart.IsZero() {
		node.ValidFrom = graphFirstStart
	}
	if !graphFirstEnd.IsZero() {
		node.ValidTo = graphFirstEnd
	}
	return node, graphFirstApplied
}

func applyCompileNodeProjection(node memory.AcceptedNode, graphNodesByID map[string]compile.GraphNode, graphFirstApplied bool) memory.AcceptedNode {
	derived, ok := graphNodesByID[node.NodeID]
	if !ok {
		return node
	}
	sameMeaning := sameNodeMeaning(node.NodeText, derived.Text) || strings.TrimSpace(node.NodeText) == strings.TrimSpace(derived.Text)
	if !graphFirstApplied && !sameMeaning {
		return node
	}
	if derivedText := strings.TrimSpace(derived.Text); derivedText != "" && (!graphFirstApplied || sameNodeMeaning(node.NodeText, derived.Text)) {
		node.NodeText = derivedText
	}
	if derivedKind := strings.TrimSpace(string(derived.Kind)); derivedKind != "" {
		node.NodeKind = derivedKind
	}
	derivedStart, derivedEnd := derived.LegacyValidityWindow()
	if node.ValidFrom.IsZero() {
		node.ValidFrom = derivedStart
	}
	if node.ValidTo.IsZero() {
		node.ValidTo = derivedEnd
	}
	return node
}
