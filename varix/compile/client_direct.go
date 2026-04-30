package compile

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

func (c *Client) compileDirect(ctx context.Context, bundle Bundle) (Output, RecordMetrics, error) {
	debugCompileStage(bundle, "compile_direct", "start")
	totalStart := time.Now()
	stageMetrics := map[string]int64{}
	generated, err := c.runCompileDirectStage(ctx, bundle, stageMetrics, "unified_generator", func() (UnifiedCompileOutput, error) {
		return c.compileUnifiedGenerate(ctx, bundle)
	})
	if err != nil {
		return Output{}, RecordMetrics{}, err
	}
	challenged, err := c.runCompileDirectStage(ctx, bundle, stageMetrics, "unified_challenge", func() (UnifiedCompileOutput, error) {
		return c.compileUnifiedChallenge(ctx, bundle, generated)
	})
	if err != nil {
		return Output{}, RecordMetrics{}, err
	}
	final, err := c.runCompileDirectStage(ctx, bundle, stageMetrics, "unified_judge", func() (UnifiedCompileOutput, error) {
		return c.compileUnifiedJudge(ctx, bundle, generated, challenged)
	})
	if err != nil {
		return Output{}, RecordMetrics{}, err
	}
	return mergeUnifiedCompileOutput(bundle, final), RecordMetrics{
		CompileElapsedMS:      DurationToMilliseconds(time.Since(totalStart)),
		CompileStageElapsedMS: stageMetrics,
	}, nil
}

func (c *Client) runCompileDirectStage(ctx context.Context, bundle Bundle, stageMetrics map[string]int64, stageName string, run func() (UnifiedCompileOutput, error)) (UnifiedCompileOutput, error) {
	stageStart := time.Now()
	out, err := run()
	if err != nil {
		debugCompileStage(bundle, "compile_direct", stageName+"_error: "+err.Error())
		return UnifiedCompileOutput{}, err
	}
	recordCompileStageMetric(stageMetrics, stageName, time.Since(stageStart))
	debugCompileStage(bundle, "compile_direct", stageName+"_done")
	return out, nil
}

func recordCompileStageMetric(metrics map[string]int64, stage string, duration time.Duration) {
	if metrics == nil {
		return
	}
	metrics[stage] = DurationToMilliseconds(duration)
}

func (c *Client) compileUnifiedGenerate(ctx context.Context, bundle Bundle) (UnifiedCompileOutput, error) {
	systemPrompt, err := c.prompts.buildUnifiedGeneratorInstruction()
	if err != nil {
		return UnifiedCompileOutput{}, err
	}
	userPrompt, err := c.prompts.buildUnifiedGeneratorPrompt(bundle)
	if err != nil {
		return UnifiedCompileOutput{}, err
	}
	return c.executeUnifiedStage(ctx, bundle, "unified_generator", systemPrompt, userPrompt, sanitizeUnifiedGeneratorOrJudgeOutput, UnifiedCompileOutput.ValidateGeneratorOrJudge)
}

func (c *Client) compileUnifiedChallenge(ctx context.Context, bundle Bundle, generated UnifiedCompileOutput) (UnifiedCompileOutput, error) {
	systemPrompt, err := c.prompts.buildUnifiedChallengeInstruction()
	if err != nil {
		return UnifiedCompileOutput{}, err
	}
	userPrompt, err := c.prompts.buildUnifiedChallengePrompt(bundle, generated)
	if err != nil {
		return UnifiedCompileOutput{}, err
	}
	out, err := c.executeUnifiedStage(ctx, bundle, "unified_challenge", systemPrompt, userPrompt, sanitizeUnifiedChallengeOutput, UnifiedCompileOutput.ValidateChallenge)
	if err != nil {
		debugCompileStage(bundle, "unified_challenge", "degrading_to_empty_corrections_after_error")
		return emptyUnifiedChallengeOutput(), nil
	}
	return out, nil
}

func (c *Client) compileUnifiedJudge(ctx context.Context, bundle Bundle, generated UnifiedCompileOutput, challenged UnifiedCompileOutput) (UnifiedCompileOutput, error) {
	systemPrompt, err := c.prompts.buildUnifiedJudgeInstruction()
	if err != nil {
		return UnifiedCompileOutput{}, err
	}
	userPrompt, err := c.prompts.buildUnifiedJudgePrompt(bundle, generated, challenged)
	if err != nil {
		return UnifiedCompileOutput{}, err
	}
	return c.executeUnifiedStage(ctx, bundle, "unified_judge", systemPrompt, userPrompt, sanitizeUnifiedGeneratorOrJudgeOutput, UnifiedCompileOutput.ValidateGeneratorOrJudge)
}

func (c *Client) executeUnifiedStage(
	ctx context.Context,
	bundle Bundle,
	stageName string,
	systemPrompt string,
	userPrompt string,
	sanitize func(UnifiedCompileOutput) UnifiedCompileOutput,
	validate func(UnifiedCompileOutput) error,
) (UnifiedCompileOutput, error) {
	out, err := c.buildUnifiedAttempt(ctx, bundle, stageName, systemPrompt, userPrompt)
	if err != nil {
		return UnifiedCompileOutput{}, err
	}
	out = sanitize(out)
	if err := validate(out); err != nil {
		return UnifiedCompileOutput{}, err
	}
	return out, nil
}

func (c *Client) buildUnifiedAttempt(ctx context.Context, bundle Bundle, stageName string, systemPrompt string, userPrompt string) (UnifiedCompileOutput, error) {
	debugCompileStage(bundle, stageName, "start")
	req, err := BuildQwen36ProviderRequest(c.model, bundle, systemPrompt, userPrompt)
	if err != nil {
		debugCompileStage(bundle, stageName, "build_request_error: "+err.Error())
		return UnifiedCompileOutput{}, err
	}
	resp, err := c.runtime.Call(ctx, req)
	if err != nil {
		debugCompileStage(bundle, stageName, "call_error: "+err.Error())
		return UnifiedCompileOutput{}, err
	}
	debugCompileStage(bundle, stageName, fmt.Sprintf("response_received model=%s bytes=%d", strings.TrimSpace(resp.Model), len(resp.Text)))
	out, err := ParseUnifiedCompileOutput(resp.Text)
	if err != nil {
		debugCompileStage(bundle, stageName, "parse_error: "+err.Error())
		if strings.TrimSpace(os.Getenv("COMPILE_STAGE_DEBUG")) != "" {
			snippet := resp.Text
			if len(snippet) > 4000 {
				snippet = snippet[:4000]
			}
			fmt.Fprintf(os.Stderr, "[compile-stage] %s raw_response_begin\n%s\n[compile-stage] %s raw_response_end\n", stageName, snippet, stageName)
		}
		return UnifiedCompileOutput{}, err
	}
	debugCompileStage(bundle, stageName, fmt.Sprintf("done drivers=%d targets=%d paths=%d evidence=%d explanation=%d", len(out.Drivers), len(out.Targets), len(out.TransmissionPaths), len(out.EvidenceNodes), len(out.ExplanationNodes)))
	return out, nil
}

func debugCompileStage(bundle Bundle, stageName string, message string) {
	if strings.TrimSpace(os.Getenv("COMPILE_STAGE_DEBUG")) == "" {
		return
	}
	unitID := strings.TrimSpace(bundle.UnitID)
	if unitID == "" {
		unitID = strings.TrimSpace(bundle.ExternalID)
	}
	fmt.Fprintf(os.Stderr, "[compile-stage] %s %s %s\n", NowUTC().Format(time.RFC3339), stageName, unitID)
	fmt.Fprintf(os.Stderr, "[compile-stage] %s %s\n", stageName, message)
}

func mergeUnifiedCompileOutput(bundle Bundle, final UnifiedCompileOutput) Output {
	driverTarget := DriverTargetOutput{
		Drivers:    final.Drivers,
		Targets:    final.Targets,
		Details:    final.Details,
		Topics:     final.Topics,
		Confidence: final.Confidence,
	}
	paths := TransmissionPathOutput{
		TransmissionPaths: final.TransmissionPaths,
		Details:           final.Details,
		Topics:            final.Topics,
		Confidence:        final.Confidence,
	}
	aux := EvidenceExplanationOutput{
		EvidenceNodes:      final.EvidenceNodes,
		ExplanationNodes:   final.ExplanationNodes,
		SupplementaryNodes: final.SupplementaryNodes,
		Details:            final.Details,
		Topics:             final.Topics,
		Confidence:         final.Confidence,
	}
	graph := buildCompatibilityGraph(bundle, driverTarget, paths, aux)
	return Output{
		Summary:            strings.TrimSpace(final.Summary),
		Drivers:            driverTarget.Drivers,
		Targets:            driverTarget.Targets,
		TransmissionPaths:  paths.TransmissionPaths,
		EvidenceNodes:      aux.EvidenceNodes,
		ExplanationNodes:   aux.ExplanationNodes,
		SupplementaryNodes: aux.SupplementaryNodes,
		Graph:              graph,
		Details:            final.Details,
		Topics:             final.Topics,
		Confidence:         final.Confidence,
	}
}

func sanitizeUnifiedChallengeOutput(out UnifiedCompileOutput) UnifiedCompileOutput {
	out = sanitizeUnifiedCommonOutput(out)
	sanitizedPaths := make([]TransmissionPath, 0, len(out.TransmissionPaths))
	for _, path := range out.TransmissionPaths {
		if path.Driver == "" || path.Target == "" || len(path.Steps) == 0 {
			continue
		}
		sanitizedPaths = append(sanitizedPaths, path)
	}
	out.TransmissionPaths = sanitizedPaths
	return out
}

func sanitizeUnifiedGeneratorOrJudgeOutput(out UnifiedCompileOutput) UnifiedCompileOutput {
	out = sanitizeUnifiedCommonOutput(out)
	sanitizedPaths := make([]TransmissionPath, 0, len(out.TransmissionPaths))
	for _, path := range out.TransmissionPaths {
		switch {
		case path.Driver != "" && path.Target != "" && len(path.Steps) > 0:
			sanitizedPaths = append(sanitizedPaths, path)
		case path.Driver == "" && path.Target != "" && len(path.Steps) > 0 && len(out.Drivers) == 1:
			path.Driver = out.Drivers[0]
			sanitizedPaths = append(sanitizedPaths, path)
		case path.Driver != "" && path.Target == "" && len(path.Steps) > 0 && len(out.Targets) == 1:
			path.Target = out.Targets[0]
			sanitizedPaths = append(sanitizedPaths, path)
		}
	}
	sanitizedPaths = alignTransmissionPathDrivers(out.Drivers, sanitizedPaths)
	if len(sanitizedPaths) == 0 && len(out.Drivers) > 0 && len(out.Targets) > 0 {
		sanitizedPaths = append(sanitizedPaths, TransmissionPath{
			Driver: out.Drivers[0],
			Target: out.Targets[0],
			Steps:  []string{out.Drivers[0]},
		})
	}
	out.TransmissionPaths = sanitizedPaths
	return out
}

func alignTransmissionPathDrivers(drivers []string, paths []TransmissionPath) []TransmissionPath {
	if len(drivers) == 0 || len(paths) == 0 {
		return paths
	}
	driverIndex := make(map[string]string, len(drivers))
	for _, driver := range drivers {
		driverIndex[normalizeLooseText(driver)] = strings.TrimSpace(driver)
	}
	aligned := make([]TransmissionPath, 0, len(paths))
	for _, path := range paths {
		normalizedPathDriver := normalizeLooseText(path.Driver)
		if exact, ok := driverIndex[normalizedPathDriver]; ok {
			path.Driver = exact
			aligned = append(aligned, path)
			continue
		}
		matches := make([]string, 0, len(drivers))
		for _, driver := range drivers {
			if pathDriverContainsDriver(path.Driver, driver) {
				matches = append(matches, strings.TrimSpace(driver))
			}
		}
		if len(matches) == 0 {
			aligned = append(aligned, path)
			continue
		}
		for _, driver := range matches {
			cloned := path
			cloned.Driver = driver
			cloned.Steps = CloneStrings(path.Steps)
			aligned = append(aligned, cloned)
		}
	}
	return dedupeTransmissionPaths(aligned)
}

func pathDriverContainsDriver(pathDriver, driver string) bool {
	pathDriver = strings.TrimSpace(pathDriver)
	driver = strings.TrimSpace(driver)
	if pathDriver == "" || driver == "" {
		return false
	}
	if normalizeLooseText(pathDriver) == normalizeLooseText(driver) {
		return true
	}
	if !containsParallelConnector(pathDriver) {
		return false
	}
	return strings.Contains(normalizeLooseText(pathDriver), normalizeLooseText(driver))
}

func containsParallelConnector(text string) bool {
	for _, connector := range []string{"以及", "并且", "同时", "和", "及", "与", "且", " and "} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(connector)) {
			return true
		}
	}
	return false
}

func dedupeTransmissionPaths(paths []TransmissionPath) []TransmissionPath {
	seen := map[string]struct{}{}
	out := make([]TransmissionPath, 0, len(paths))
	for _, path := range paths {
		key := normalizeLooseText(path.Driver) + "|" + normalizeLooseText(path.Target) + "|" + normalizeLooseText(strings.Join(path.Steps, " "))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, path)
	}
	return out
}

func sanitizeUnifiedCommonOutput(out UnifiedCompileOutput) UnifiedCompileOutput {
	out.Summary = strings.TrimSpace(out.Summary)
	out.Drivers = normalizeStringList(out.Drivers)
	out.Targets = normalizeStringList(out.Targets)
	out.EvidenceNodes = normalizeStringList(out.EvidenceNodes)
	out.ExplanationNodes = normalizeStringList(out.ExplanationNodes)
	out.SupplementaryNodes = normalizeStringList(out.SupplementaryNodes)
	if out.Details.IsEmpty() {
		out.Details = HiddenDetails{Caveats: []string{"model omitted details"}}
	}
	for i := range out.TransmissionPaths {
		out.TransmissionPaths[i].Driver = strings.TrimSpace(out.TransmissionPaths[i].Driver)
		out.TransmissionPaths[i].Target = strings.TrimSpace(out.TransmissionPaths[i].Target)
		out.TransmissionPaths[i].Steps = normalizeStringList(out.TransmissionPaths[i].Steps)
	}
	return out
}

func emptyUnifiedChallengeOutput() UnifiedCompileOutput {
	return UnifiedCompileOutput{
		Summary: "no reliable challenge corrections",
		Details: HiddenDetails{Caveats: []string{"challenge unavailable"}},
	}
}

func summarizeDirectCompileOutput(driverTarget DriverTargetOutput, paths TransmissionPathOutput) string {
	if len(paths.TransmissionPaths) > 0 {
		path := paths.TransmissionPaths[0]
		driver := strings.TrimSpace(path.Driver)
		target := strings.TrimSpace(path.Target)
		if driver != "" && target != "" {
			return driver + "，并最终推动" + target
		}
	}
	if len(driverTarget.Drivers) > 0 && len(driverTarget.Targets) > 0 {
		driver := strings.TrimSpace(driverTarget.Drivers[0])
		target := strings.TrimSpace(driverTarget.Targets[0])
		return driver + "，并推动" + target
	}
	if len(driverTarget.Targets) > 0 {
		return strings.TrimSpace(driverTarget.Targets[0])
	}
	return "compile summary unavailable"
}
