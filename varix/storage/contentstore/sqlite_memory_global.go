package contentstore

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

func (s *SQLiteStore) RunGlobalMemoryOrganization(ctx context.Context, userID string, now time.Time) (memory.GlobalOrganizationOutput, error) {
	var err error
	userID, err = normalizeRequiredUserID(userID)
	if err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}
	now = normalizeNow(now)

	nodes, err := s.ListUserMemory(ctx, userID)
	if err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}

	globalNodes := globalizeAcceptedNodes(nodes)
	active, inactive := splitAcceptedNodesByActivity(globalNodes, now)

	dedupeGroups := buildDedupeGroups(active, nil, nil)
	contradictionGroups := buildContradictionGroups(active)
	clusters := buildGlobalClusters(active, dedupeGroups, contradictionGroups, now)
	openQuestions := buildGlobalOpenQuestions(clusters)

	output := memory.GlobalOrganizationOutput{
		UserID:              userID,
		GeneratedAt:         now,
		ActiveNodes:         active,
		InactiveNodes:       inactive,
		DedupeGroups:        dedupeGroups,
		ContradictionGroups: contradictionGroups,
		Clusters:            clusters,
		OpenQuestions:       openQuestions,
	}

	if err := persistLatestUserScopedOutput(ctx, s.db, "global_memory_organization_outputs", output.UserID, now, &output, func(id int64) {
		output.OutputID = id
	}); err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}
	return output, nil
}
func (s *SQLiteStore) GetLatestGlobalMemoryOrganizationOutput(ctx context.Context, userID string) (memory.GlobalOrganizationOutput, error) {
	payload, err := latestUserScopedPayload(ctx, s.db, "global_memory_organization_outputs", userID)
	if err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}
	var out memory.GlobalOrganizationOutput
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}
	return out, nil
}
