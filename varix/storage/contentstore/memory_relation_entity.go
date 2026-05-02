package contentstore

import (
	"fmt"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
	"strings"
	"time"
)

func relationBoundaryLabels(thesis memory.CausalThesis, compiledNodes map[string]model.GraphNode) (driverLabel, targetLabel string, ok bool) {
	if len(thesis.CorePathNodeIDs) == 0 {
		return "", "", false
	}
	driverLabel = relationNodeLabel(thesis.CorePathNodeIDs[0], compiledNodes)
	targetLabel = relationNodeLabel(thesis.CorePathNodeIDs[len(thesis.CorePathNodeIDs)-1], compiledNodes)
	driverLabel = strings.TrimSpace(driverLabel)
	targetLabel = strings.TrimSpace(targetLabel)
	return driverLabel, targetLabel, driverLabel != "" && targetLabel != ""
}

func relationNodeLabel(globalNodeID string, compiledNodes map[string]model.GraphNode) string {
	if node, ok := compiledNodes[globalNodeID]; ok {
		return normalizeCanonicalDisplay(node.Text)
	}
	return normalizeCanonicalDisplay(extractNodeTextFromGlobalRef(globalNodeID))
}

func extractNodeTextFromGlobalRef(globalNodeID string) string {
	parts := strings.Split(globalNodeID, ":")
	if len(parts) == 0 {
		return globalNodeID
	}
	return parts[len(parts)-1]
}

func addCanonicalEntity(states map[string]*memory.CanonicalEntity, entityID string, entityType memory.CanonicalEntityType, canonicalName string, now time.Time) {
	canonicalName = normalizeCanonicalDisplay(canonicalName)
	if canonicalName == "" || strings.TrimSpace(entityID) == "" {
		return
	}
	if existing, ok := states[entityID]; ok {
		existing.EntityType = combineEntityTypes(existing.EntityType, entityType)
		existing.UpdatedAt = now
		return
	}
	states[entityID] = &memory.CanonicalEntity{
		EntityID:      entityID,
		EntityType:    entityType,
		CanonicalName: canonicalName,
		Status:        memory.CanonicalEntityActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func relationEntityID(entityType memory.CanonicalEntityType, label string) string {
	label = normalizeCanonicalAlias(label)
	if label == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", entityType, label)
}

func combineEntityTypes(left, right memory.CanonicalEntityType) memory.CanonicalEntityType {
	if left == "" {
		return right
	}
	if right == "" || left == right {
		return left
	}
	return memory.CanonicalEntityBoth
}
