package compile

type GraphRequirements struct {
	MinNodes int
	MinEdges int
}

func InferGraphRequirements(bundle Bundle) GraphRequirements {
	length := bundle.ApproxTextLength()
	switch {
	case length >= 8000:
		return GraphRequirements{MinNodes: 6, MinEdges: 5}
	case length >= 2500:
		return GraphRequirements{MinNodes: 4, MinEdges: 3}
	default:
		return GraphRequirements{MinNodes: 2, MinEdges: 1}
	}
}
