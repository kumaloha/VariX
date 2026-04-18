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

func BuildGraphInstruction(req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildGraphInstruction(req))
}

func BuildGraphPrompt(bundle Bundle, nodes []GraphNode) string {
	return mustRenderPrompt(defaultPromptRegistry().buildGraphPrompt(bundle, nodes))
}

func BuildGraphRetryPrompt(bundle Bundle, nodes []GraphNode, req GraphRequirements) string {
	return mustRenderPrompt(defaultPromptRegistry().buildGraphRetryPrompt(bundle, nodes, req))
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
