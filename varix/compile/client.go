package compile

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/config"
	"github.com/kumaloha/forge/llm"
)

const defaultDashScopeCompatibleBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

type runtimeChat interface {
	Call(ctx context.Context, req llm.ProviderRequest) (llm.Response, error)
}

type Client struct {
	runtime        runtimeChat
	model          string
	prompts        *promptRegistry
	verifier       VerificationService
	skipValidation bool
}

type noopVerificationService struct{}

func (noopVerificationService) Verify(ctx context.Context, bundle Bundle, output Output) (Verification, error) {
	return Verification{}, nil
}

func NewClient(httpClient *http.Client, baseURL, apiKey, model string) *Client {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	opts := []llm.DashscopeOption{
		llm.WithAPIKey(apiKey),
	}
	if strings.TrimSpace(baseURL) != "" {
		opts = append(opts, llm.WithAPIBase(baseURL))
	}
	if httpClient != nil && httpClient.Timeout > 0 {
		opts = append(opts, llm.WithTimeout(httpClient.Timeout))
	}
	provider, err := llm.NewDashscope(opts...)
	if err != nil {
		return nil
	}
	return NewClientWithRuntimeAndPrompts(llm.NewRuntime(llm.RuntimeConfig{
		Provider: provider,
		LLMConfig: llm.LLMConfig{
			Default: llm.DefaultConfig{
				Model:       strings.TrimSpace(model),
				Search:      false,
				Temperature: 0,
				Thinking:    false,
			},
		},
		MaxAttempts: 3,
	}), strings.TrimSpace(model), newPromptRegistry(""))
}

func NewClientWithRuntime(rt runtimeChat, model string) *Client {
	return NewClientWithRuntimeAndPrompts(rt, model, newPromptRegistry(""))
}

func NewClientWithRuntimeAndPrompts(rt runtimeChat, model string, prompts *promptRegistry) *Client {
	return NewClientWithRuntimePromptsAndVerifier(rt, model, prompts, nil)
}

func NewClientWithRuntimePromptsAndVerifier(rt runtimeChat, model string, prompts *promptRegistry, verifier VerificationService) *Client {
	return NewClientWithRuntimePromptsAndVerifierOptions(rt, model, prompts, verifier, false)
}

func NewClientWithRuntimePromptsAndVerifierOptions(rt runtimeChat, model string, prompts *promptRegistry, verifier VerificationService, skipValidation bool) *Client {
	if rt == nil {
		return nil
	}
	if prompts == nil {
		prompts = newPromptRegistry("")
	}
	if verifier == nil {
		verifier = NewVerificationService(rt, model, prompts)
	}
	return &Client{
		runtime:        rt,
		model:          strings.TrimSpace(model),
		prompts:        prompts,
		verifier:       verifier,
		skipValidation: skipValidation,
	}
}

func NewClientFromConfig(projectRoot string, httpClient *http.Client) *Client {
	return newClientFromConfig(projectRoot, httpClient, nil, false)
}

func NewClientFromConfigNoVerify(projectRoot string, httpClient *http.Client) *Client {
	return newClientFromConfig(projectRoot, httpClient, noopVerificationService{}, false)
}

func NewClientFromConfigNoVerifyNoValidate(projectRoot string, httpClient *http.Client) *Client {
	return newClientFromConfig(projectRoot, httpClient, noopVerificationService{}, true)
}

func newClientFromConfig(projectRoot string, httpClient *http.Client, verifier VerificationService, skipValidation bool) *Client {
	settings := config.DefaultSettings(projectRoot)
	apiKey := config.FirstConfiguredValue(projectRoot, "DASHSCOPE_API_KEY", "OPENAI_API_KEY")
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	baseURL := config.FirstConfiguredValue(projectRoot, "COMPILE_BASE_URL", "DASHSCOPE_BASE_URL")
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultDashScopeCompatibleBaseURL
	}
	model := config.FirstConfiguredValue(projectRoot, "COMPILE_MODEL")
	if strings.TrimSpace(model) == "" {
		model = Qwen36PlusModel
	}
	if httpClient == nil {
		timeout := 1200 * time.Second
		if raw := config.FirstConfiguredValue(projectRoot, "COMPILE_TIMEOUT_SECONDS"); strings.TrimSpace(raw) != "" {
			if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
				timeout = time.Duration(seconds) * time.Second
			}
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	opts := []llm.DashscopeOption{
		llm.WithAPIKey(apiKey),
	}
	if strings.TrimSpace(baseURL) != "" {
		opts = append(opts, llm.WithAPIBase(baseURL))
	}
	if httpClient != nil && httpClient.Timeout > 0 {
		opts = append(opts, llm.WithTimeout(httpClient.Timeout))
	}
	provider, err := llm.NewDashscope(opts...)
	if err != nil {
		return nil
	}
	runtime := llm.NewRuntime(llm.RuntimeConfig{
		Provider: provider,
		LLMConfig: llm.LLMConfig{
			Default: llm.DefaultConfig{
				Model:       strings.TrimSpace(model),
				Search:      false,
				Temperature: 0,
				Thinking:    false,
			},
		},
		MaxAttempts: 3,
	})
	client := NewClientWithRuntimePromptsAndVerifierOptions(runtime, strings.TrimSpace(model), newPromptRegistry(settings.PromptsDir), verifier, skipValidation)
	if client == nil {
		return nil
	}
	client.prompts = newPromptRegistry(settings.PromptsDir)
	return client
}

func (c *Client) Compile(ctx context.Context, bundle Bundle) (Record, error) {
	if c == nil || c.runtime == nil {
		return Record{}, fmt.Errorf("compile client is nil")
	}
	if c.prompts == nil {
		c.prompts = newPromptRegistry("")
	}
	output, metrics, err := c.compileDirect(ctx, bundle)
	if err != nil {
		return Record{}, err
	}
	return Record{
		UnitID:         bundle.UnitID,
		Source:         bundle.Source,
		ExternalID:     bundle.ExternalID,
		RootExternalID: bundle.RootExternalID,
		Model:          c.model,
		Metrics:        metrics,
		Output:         output,
		CompiledAt:     NowUTC(),
	}, nil
}

func (c *Client) Verify(ctx context.Context, bundle Bundle, output Output) (Verification, error) {
	if c == nil {
		return Verification{}, fmt.Errorf("verify client is nil")
	}
	if c.verifier == nil {
		return Verification{}, nil
	}
	return c.verifier.Verify(ctx, bundle, output)
}

func (c *Client) VerifyDetailed(ctx context.Context, bundle Bundle, output Output) (Verification, error) {
	if c == nil {
		return Verification{}, fmt.Errorf("verify client is nil")
	}
	if c.runtime == nil {
		return Verification{}, fmt.Errorf("verify client runtime unavailable")
	}
	if c.prompts == nil {
		c.prompts = newPromptRegistry("")
	}
	return runDetailedVerifier(ctx, c.runtime, c.model, c.prompts, bundle, output)
}

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
		if strings.TrimSpace(path.Driver) != "" && strings.TrimSpace(path.Target) != "" {
			return strings.TrimSpace(path.Driver) + "，并最终推动" + strings.TrimSpace(path.Target)
		}
	}
	if len(driverTarget.Drivers) > 0 && len(driverTarget.Targets) > 0 {
		return strings.TrimSpace(driverTarget.Drivers[0]) + "，并推动" + strings.TrimSpace(driverTarget.Targets[0])
	}
	if len(driverTarget.Targets) > 0 {
		return strings.TrimSpace(driverTarget.Targets[0])
	}
	return "compile summary unavailable"
}

func buildCompatibilityGraph(bundle Bundle, driverTarget DriverTargetOutput, paths TransmissionPathOutput, aux EvidenceExplanationOutput) ReasoningGraph {
	graph := ReasoningGraph{}
	keyToID := map[string]string{}
	nextID := 1
	now := bundle.PostedAt.UTC()
	if bundle.PostedAt.IsZero() {
		now = NowUTC()
	}

	addNode := func(kind NodeKind, text string) string {
		normalizedText := strings.TrimSpace(text)
		if normalizedText == "" {
			return ""
		}
		key := string(kind) + "|" + strings.ToLower(strings.Join(strings.Fields(normalizedText), " "))
		if existing, ok := keyToID[key]; ok {
			return existing
		}
		id := fmt.Sprintf("n%d", nextID)
		nextID++
		node := GraphNode{ID: id, Kind: kind, Text: normalizedText}
		switch kind {
		case NodeFact, NodeMechanism, NodeImplicitCondition:
			node.OccurredAt = now
		case NodePrediction:
			node.PredictionStartAt = now
		}
		graph.Nodes = append(graph.Nodes, node)
		keyToID[key] = id
		return id
	}

	addEdge := func(from, to string, kind EdgeKind) {
		if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" || from == to {
			return
		}
		candidate := GraphEdge{From: from, To: to, Kind: kind}
		for _, existing := range graph.Edges {
			if existing == candidate {
				return
			}
		}
		graph.Edges = append(graph.Edges, candidate)
	}

	for _, driver := range driverTarget.Drivers {
		addNode(NodeMechanism, driver)
	}

	targetNodeIDs := make([]string, 0, len(driverTarget.Targets))
	for _, target := range driverTarget.Targets {
		targetNodeIDs = append(targetNodeIDs, addNode(NodeConclusion, target))
	}
	primaryTargetID := ""
	if len(targetNodeIDs) > 0 {
		primaryTargetID = targetNodeIDs[0]
	}

	for _, path := range paths.TransmissionPaths {
		driverID := addNode(NodeMechanism, path.Driver)
		lastID := driverID
		for _, step := range path.Steps {
			stepID := addNode(NodeMechanism, step)
			if lastID != "" && stepID != "" && lastID != stepID {
				addEdge(lastID, stepID, EdgePositive)
			}
			if stepID != "" {
				lastID = stepID
			}
		}
		targetID := addNode(NodeConclusion, path.Target)
		if lastID == "" {
			lastID = driverID
		}
		addEdge(lastID, targetID, EdgePositive)
	}

	for _, evidence := range aux.EvidenceNodes {
		if hasNormalizedMechanismText(driverTarget.Drivers, paths.TransmissionPaths, evidence) {
			continue
		}
		evidenceID := addNode(NodeFact, evidence)
		if primaryTargetID != "" {
			addEdge(evidenceID, primaryTargetID, EdgeDerives)
		}
	}

	for _, explanation := range aux.ExplanationNodes {
		explanationID := addNode(NodeConclusion, explanation)
		if primaryTargetID != "" {
			addEdge(explanationID, primaryTargetID, EdgeExplains)
		}
	}

	for _, supplementary := range aux.SupplementaryNodes {
		supplementaryID := addNode(NodeConclusion, supplementary)
		if primaryTargetID != "" {
			addEdge(supplementaryID, primaryTargetID, EdgeExplains)
		}
	}

	ensureMinimumCompatibilityGraph(&graph, bundle, driverTarget, paths, aux, addNode, addEdge)
	applyBundleTimingFallbacks(bundle, &graph)
	return graph
}

func ensureMinimumCompatibilityGraph(
	graph *ReasoningGraph,
	bundle Bundle,
	driverTarget DriverTargetOutput,
	paths TransmissionPathOutput,
	aux EvidenceExplanationOutput,
	addNode func(kind NodeKind, text string) string,
	addEdge func(from, to string, kind EdgeKind),
) {
	if graph == nil {
		return
	}
	if len(graph.Nodes) >= 2 && len(graph.Edges) >= 1 {
		return
	}

	driverText := firstNonEmptyTrimmed(
		firstString(driverTarget.Drivers),
		firstPathDriver(paths.TransmissionPaths),
		firstString(aux.ExplanationNodes),
		strings.TrimSpace(bundle.Content),
		"primary driver",
	)
	targetText := firstNonEmptyTrimmed(
		firstString(driverTarget.Targets),
		firstPathTarget(paths.TransmissionPaths),
		firstString(aux.EvidenceNodes),
		strings.TrimSpace(bundle.Content),
		"primary target",
	)

	driverID := addNode(NodeMechanism, driverText)
	targetID := addNode(NodeConclusion, targetText)
	addEdge(driverID, targetID, EdgePositive)

	if len(graph.Nodes) < 2 {
		evidenceID := addNode(NodeFact, firstNonEmptyTrimmed(firstString(aux.EvidenceNodes), targetText, "supporting evidence"))
		addEdge(evidenceID, targetID, EdgeDerives)
	}
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstPathDriver(paths []TransmissionPath) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0].Driver
}

func firstPathTarget(paths []TransmissionPath) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0].Target
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func appendUniqueNormalized(base []string, values ...string) []string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		found := false
		for _, existing := range base {
			if normalizeLooseText(existing) == normalizeLooseText(value) {
				found = true
				break
			}
		}
		if !found {
			base = append(base, value)
		}
	}
	return base
}

func findNodeText(nodes []GraphNode, id string) string {
	for _, node := range nodes {
		if node.ID == id {
			return strings.TrimSpace(node.Text)
		}
	}
	return ""
}

func firstNormalizedOrFallback(values []string, fallback string) string {
	if len(values) > 0 && strings.TrimSpace(values[0]) != "" {
		return strings.TrimSpace(values[0])
	}
	return strings.TrimSpace(fallback)
}

func normalizeLooseText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func hasNormalizedMechanismText(drivers []string, paths []TransmissionPath, evidence string) bool {
	target := normalizeLooseText(evidence)
	for _, driver := range drivers {
		if normalizeLooseText(driver) == target {
			return true
		}
	}
	for _, path := range paths {
		for _, step := range path.Steps {
			if normalizeLooseText(step) == target {
				return true
			}
		}
	}
	return false
}

func mergeCompileOutputs(nodes NodeExtractionOutput, fullGraph FullGraphOutput, thesis ThesisOutput) Output {
	topics := thesis.Topics
	if len(topics) == 0 {
		topics = fullGraph.Topics
	}
	if len(topics) == 0 {
		topics = nodes.Topics
	}
	confidence := strings.TrimSpace(thesis.Confidence)
	if confidence == "" {
		confidence = strings.TrimSpace(fullGraph.Confidence)
	}
	if confidence == "" {
		confidence = strings.TrimSpace(nodes.Confidence)
	}
	details := thesis.Details
	if details.IsEmpty() {
		details = fullGraph.Details
	}
	if details.IsEmpty() {
		details = nodes.Details
	}
	return Output{
		Summary:    thesis.Summary,
		Drivers:    thesis.Drivers,
		Targets:    thesis.Targets,
		Graph:      ReasoningGraph{Nodes: nodes.Graph.Nodes, Edges: fullGraph.Graph.Edges},
		Details:    details,
		Topics:     topics,
		Confidence: confidence,
	}
}

func mergeNodeOutputs(base NodeExtractionOutput, challenge NodeExtractionOutput) NodeExtractionOutput {
	if len(challenge.Graph.Nodes) == 0 {
		return base
	}
	out := base
	seen := map[string]struct{}{}
	usedIDs := map[string]struct{}{}
	for _, node := range out.Graph.Nodes {
		seen[nodeDedupKey(node)] = struct{}{}
		usedIDs[node.ID] = struct{}{}
	}
	next := 1
	for _, node := range challenge.Graph.Nodes {
		key := nodeDedupKey(node)
		if _, ok := seen[key]; ok {
			continue
		}
		if strings.TrimSpace(node.ID) == "" {
			node.ID = ""
		}
		for strings.TrimSpace(node.ID) == "" || hasStringKey(usedIDs, node.ID) {
			node.ID = fmt.Sprintf("n_challenge_%d", next)
			next++
		}
		usedIDs[node.ID] = struct{}{}
		seen[key] = struct{}{}
		out.Graph.Nodes = append(out.Graph.Nodes, node)
	}
	if out.Details.IsEmpty() {
		out.Details = challenge.Details
	}
	if len(out.Topics) == 0 {
		out.Topics = challenge.Topics
	}
	if strings.TrimSpace(out.Confidence) == "" {
		out.Confidence = challenge.Confidence
	}
	return out
}

func mergeFullGraphOutputs(base FullGraphOutput, challenge FullGraphOutput) FullGraphOutput {
	if len(challenge.Graph.Edges) == 0 {
		return base
	}
	out := base
	byPair := map[string]int{}
	for i, edge := range out.Graph.Edges {
		byPair[edgePairKey(edge)] = i
	}
	for _, edge := range challenge.Graph.Edges {
		if idx, ok := byPair[edgePairKey(edge)]; ok {
			out.Graph.Edges[idx] = edge
			continue
		}
		byPair[edgePairKey(edge)] = len(out.Graph.Edges)
		out.Graph.Edges = append(out.Graph.Edges, edge)
	}
	if out.Details.IsEmpty() {
		out.Details = challenge.Details
	}
	if len(out.Topics) == 0 {
		out.Topics = challenge.Topics
	}
	if strings.TrimSpace(out.Confidence) == "" {
		out.Confidence = challenge.Confidence
	}
	return out
}

func nodeDedupKey(node GraphNode) string {
	return string(node.Kind) + "|" + strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(node.Text)), " "))
}

func edgePairKey(edge GraphEdge) string {
	return edge.From + "->" + edge.To
}

func hasStringKey(m map[string]struct{}, key string) bool {
	_, ok := m[key]
	return ok
}

func buildCausalProjection(nodes []GraphNode, edges []GraphEdge) ReasoningGraph {
	if len(nodes) == 0 || len(edges) == 0 {
		return ReasoningGraph{}
	}
	selectedEdges := make([]GraphEdge, 0, len(edges))
	selectedIDs := map[string]struct{}{}
	for _, edge := range edges {
		if edge.Kind != EdgePositive {
			continue
		}
		selectedEdges = append(selectedEdges, edge)
		selectedIDs[edge.From] = struct{}{}
		selectedIDs[edge.To] = struct{}{}
	}
	if len(selectedEdges) == 0 {
		return ReasoningGraph{}
	}
	selectedNodes := make([]GraphNode, 0, len(selectedIDs))
	for _, node := range nodes {
		if _, ok := selectedIDs[node.ID]; ok {
			selectedNodes = append(selectedNodes, node)
		}
	}
	return ReasoningGraph{Nodes: selectedNodes, Edges: selectedEdges}
}

func applyBundleTimingFallbacks(bundle Bundle, graph *ReasoningGraph) {
	if graph == nil || bundle.PostedAt.IsZero() {
		return
	}
	fallback := bundle.PostedAt.UTC()
	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		switch node.Kind {
		case NodeFact, NodeImplicitCondition, NodeMechanism:
			if node.OccurredAt.IsZero() && node.ValidFrom.IsZero() {
				node.OccurredAt = fallback
			}
		case NodePrediction:
			if node.PredictionStartAt.IsZero() && node.ValidFrom.IsZero() {
				node.PredictionStartAt = fallback
			}
		}
	}
}
