package compile

func BuildInstruction(req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildInstruction(req))
}

func BuildRetryPrompt(bundle Bundle, req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildRetryPrompt(bundle, req))
}

func BuildPrompt(bundle Bundle) string {
	return mustRenderPrompt(defaultPromptRegistry().buildPrompt(bundle))
}

func BuildThesisInstruction(req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildThesisInstruction(req))
}

func BuildThesisPrompt(bundle Bundle, projection ReasoningGraph) string {
	return mustRenderPrompt(defaultPromptRegistry().buildThesisPrompt(bundle, projection))
}

func BuildThesisRetryPrompt(bundle Bundle, projection ReasoningGraph, req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildThesisRetryPrompt(bundle, projection, req))
}

func BuildNodeInstruction(req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildNodeInstruction(req))
}

func BuildNodePrompt(bundle Bundle) string {
	return mustRenderPrompt(defaultPromptRegistry().buildNodePrompt(bundle))
}

func BuildNodeRetryPrompt(bundle Bundle, req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildNodeRetryPrompt(bundle, req))
}

func BuildNodeChallengeInstruction(req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildNodeChallengeInstruction(req))
}

func BuildNodeChallengePrompt(bundle Bundle, nodes []GraphNode) string {
	return mustRenderPrompt(defaultPromptRegistry().buildNodeChallengePrompt(bundle, nodes))
}

func BuildNodeChallengeRetryPrompt(bundle Bundle, nodes []GraphNode, req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildNodeChallengeRetryPrompt(bundle, nodes, req))
}

func BuildGraphInstruction(req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildGraphInstruction(req))
}

func BuildGraphPrompt(bundle Bundle, nodes []GraphNode) string {
	return mustRenderPrompt(defaultPromptRegistry().buildGraphPrompt(bundle, nodes))
}

func BuildGraphRetryPrompt(bundle Bundle, nodes []GraphNode, req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildGraphRetryPrompt(bundle, nodes, req))
}

func BuildEdgeChallengeInstruction(req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildEdgeChallengeInstruction(req))
}

func BuildEdgeChallengePrompt(bundle Bundle, nodes []GraphNode, edges []GraphEdge) string {
	return mustRenderPrompt(defaultPromptRegistry().buildEdgeChallengePrompt(bundle, nodes, edges))
}

func BuildEdgeChallengeRetryPrompt(bundle Bundle, nodes []GraphNode, edges []GraphEdge, req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildEdgeChallengeRetryPrompt(bundle, nodes, edges, req))
}

func defaultPromptRegistry() *promptRegistry {
	return newPromptRegistry("")
}

func mustRenderPrompt(rendered string, err error) string {
	if err != nil {
		panic(err)
	}
	return rendered
}
