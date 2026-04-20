package graphmodel

type CompileCard struct {
	CardID         string     `json:"card_id"`
	ArticleID      string     `json:"article_id"`
	Title          string     `json:"title"`
	Summary        string     `json:"summary"`
	PrimaryNodeIDs []string   `json:"primary_node_ids,omitempty"`
	PrimaryEdgeIDs []string   `json:"primary_edge_ids,omitempty"`
	MainPaths      [][]string `json:"main_paths,omitempty"`
	EvidenceRefs   []string   `json:"evidence_refs,omitempty"`
	CompactView    bool       `json:"compact_view"`
}
