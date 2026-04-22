package graphmodel

import (
	"fmt"
	"strings"
	"time"
)

type ContentSubgraph struct {
	ID               string      `json:"id"`
	ArticleID        string      `json:"article_id"`
	SourcePlatform   string      `json:"source_platform"`
	SourceExternalID string      `json:"source_external_id"`
	RootExternalID   string      `json:"root_external_id,omitempty"`
	Nodes            []GraphNode `json:"nodes"`
	Edges            []GraphEdge `json:"edges"`
	CompileVersion   string      `json:"compile_version"`
	CompiledAt       string      `json:"compiled_at"`
	UpdatedAt        string      `json:"updated_at"`
}

func (g ContentSubgraph) Validate() error {
	if err := requireTrimmed("content subgraph id", g.ID); err != nil {
		return err
	}
	if err := requireTrimmed("content subgraph article_id", g.ArticleID); err != nil {
		return err
	}
	if err := requireTrimmed("content subgraph source_platform", g.SourcePlatform); err != nil {
		return err
	}
	if err := requireTrimmed("content subgraph source_external_id", g.SourceExternalID); err != nil {
		return err
	}
	if err := requireTrimmed("content subgraph compile_version", g.CompileVersion); err != nil {
		return err
	}
	if err := validateRequiredRFC3339("compiled_at", g.CompiledAt); err != nil {
		return err
	}
	if err := validateRequiredRFC3339("updated_at", g.UpdatedAt); err != nil {
		return err
	}
	if len(g.Nodes) == 0 {
		return fmt.Errorf("content subgraph must contain at least one node")
	}
	nodeIDs := make(map[string]struct{}, len(g.Nodes))
	for _, node := range g.Nodes {
		if err := node.Validate(); err != nil {
			return err
		}
		if _, ok := nodeIDs[node.ID]; ok {
			return fmt.Errorf("duplicate graph node id %q", node.ID)
		}
		nodeIDs[node.ID] = struct{}{}
	}
	seenEdges := map[string]struct{}{}
	for _, edge := range g.Edges {
		if err := edge.Validate(nodeIDs); err != nil {
			return err
		}
		if _, ok := seenEdges[edge.ID]; ok {
			return fmt.Errorf("duplicate graph edge id %q", edge.ID)
		}
		seenEdges[edge.ID] = struct{}{}
	}
	return nil
}

func validateRequiredRFC3339(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	return validateOptionalRFC3339(field, value)
}

func requireTrimmed(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}

func validateOptionalRFC3339(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("%s must be RFC3339: %w", field, err)
	}
	return nil
}
