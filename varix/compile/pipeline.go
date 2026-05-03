package compile

type graphRole string

const (
	roleDriver       graphRole = "driver"
	roleTransmission graphRole = "transmission"
	roleOrphan       graphRole = "orphan"
)

type graphNode struct {
	ID            string
	Text          string
	SourceQuote   string
	Role          graphRole
	DiscourseRole string
	Ontology      string
	IsTarget      bool
}

type graphEdge struct {
	From        string
	To          string
	Kind        string
	SourceQuote string
	Reason      string
}

type auxEdge struct {
	From        string
	To          string
	Kind        string
	SourceQuote string
	Reason      string
}

type offGraphItem struct {
	ID          string
	Text        string
	Role        string
	AttachesTo  string
	SourceQuote string
}

type graphState struct {
	Nodes         []graphNode
	Edges         []graphEdge
	AuxEdges      []auxEdge
	OffGraph      []offGraphItem
	BranchHeads   []string
	Spines        []PreviewSpine
	SemanticUnits []SemanticUnit
	ArticleForm   string
	Rounds        int
}

type relationKind string

const (
	relationCausal      relationKind = "causal"
	relationSupports    relationKind = "supports"
	relationSupplements relationKind = "supplements"
	relationExplains    relationKind = "explains"
	relationNone        relationKind = "none"
)

func countRole(state graphState, role graphRole) int {
	count := 0
	for _, n := range state.Nodes {
		if n.Role == role {
			count++
		}
	}
	return count
}

func countTargets(state graphState) int {
	count := 0
	for _, n := range state.Nodes {
		if n.IsTarget {
			count++
		}
	}
	return count
}
