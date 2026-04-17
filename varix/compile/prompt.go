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

func defaultPromptRegistry() *promptRegistry {
	return newPromptRegistry("")
}

func mustRenderPrompt(rendered string, err error) string {
	if err != nil {
		panic(err)
	}
	return rendered
}
