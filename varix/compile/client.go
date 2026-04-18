package compile

import (
	"context"
	"fmt"
	"net/http"
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
	apiKey := firstConfiguredValue(projectRoot, "DASHSCOPE_API_KEY", "OPENAI_API_KEY")
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	baseURL := firstConfiguredValue(projectRoot, "COMPILE_BASE_URL", "DASHSCOPE_BASE_URL")
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultDashScopeCompatibleBaseURL
	}
	model := firstConfiguredValue(projectRoot, "COMPILE_MODEL")
	if strings.TrimSpace(model) == "" {
		model = Qwen36PlusModel
	}
	if httpClient == nil {
		timeout := 180 * time.Second
		if raw := firstConfiguredValue(projectRoot, "COMPILE_TIMEOUT_SECONDS"); strings.TrimSpace(raw) != "" {
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
	driverTargetOutput, err := c.compileDriverTarget(ctx, bundle)
	if err != nil {
		return Record{}, err
	}
	transmissionPathOutput, err := c.compileTransmissionPaths(ctx, bundle, driverTargetOutput)
	if err != nil {
		return Record{}, err
	}
	evidenceExplanationOutput, err := c.compileEvidenceExplanation(ctx, bundle, driverTargetOutput, transmissionPathOutput)
	if err != nil {
		return Record{}, err
	}
	output := mergeDirectCompileOutputs(bundle, driverTargetOutput, transmissionPathOutput, evidenceExplanationOutput)
	verification, err := c.verifier.Verify(ctx, bundle, output)
	if err != nil {
		return Record{}, err
	}
	output = projectVerification(output, verification)
	return Record{
		UnitID:         bundle.UnitID,
		Source:         bundle.Source,
		ExternalID:     bundle.ExternalID,
		RootExternalID: bundle.RootExternalID,
		Model:          c.model,
		Output:         output,
		CompiledAt:     time.Now().UTC(),
	}, nil
}

func (c *Client) compileDriverTarget(ctx context.Context, bundle Bundle) (DriverTargetOutput, error) {
	systemPrompt, err := c.prompts.buildDriverTargetGeneratorInstruction()
	if err != nil {
		return DriverTargetOutput{}, err
	}
	userPrompt, err := c.prompts.buildDriverTargetGeneratorPrompt(bundle)
	if err != nil {
		return DriverTargetOutput{}, err
	}
	generated, err := c.buildDriverTargetAttempt(ctx, bundle, systemPrompt, userPrompt)
	if err != nil {
		return DriverTargetOutput{}, err
	}
	if err := generated.ValidateGeneratorOrJudge(); err != nil {
		return DriverTargetOutput{}, err
	}

	challengeSystemPrompt, err := c.prompts.buildDriverTargetChallengeInstruction()
	if err != nil {
		return DriverTargetOutput{}, err
	}
	challengeUserPrompt, err := c.prompts.buildDriverTargetChallengePrompt(bundle, generated)
	if err != nil {
		return DriverTargetOutput{}, err
	}
	challenged, err := c.buildDriverTargetAttempt(ctx, bundle, challengeSystemPrompt, challengeUserPrompt)
	if err != nil {
		return DriverTargetOutput{}, err
	}
	if err := challenged.ValidateChallenge(); err != nil {
		return DriverTargetOutput{}, err
	}

	judgeSystemPrompt, err := c.prompts.buildDriverTargetJudgeInstruction()
	if err != nil {
		return DriverTargetOutput{}, err
	}
	judgeUserPrompt, err := c.prompts.buildDriverTargetJudgePrompt(bundle, generated, challenged)
	if err != nil {
		return DriverTargetOutput{}, err
	}
	final, err := c.buildDriverTargetAttempt(ctx, bundle, judgeSystemPrompt, judgeUserPrompt)
	if err != nil {
		return DriverTargetOutput{}, err
	}
	if err := final.ValidateGeneratorOrJudge(); err != nil {
		return DriverTargetOutput{}, err
	}
	return final, nil
}

func (c *Client) compileTransmissionPaths(ctx context.Context, bundle Bundle, driverTarget DriverTargetOutput) (TransmissionPathOutput, error) {
	systemPrompt, err := c.prompts.buildTransmissionPathGeneratorInstruction()
	if err != nil {
		return TransmissionPathOutput{}, err
	}
	userPrompt, err := c.prompts.buildTransmissionPathGeneratorPrompt(bundle, driverTarget)
	if err != nil {
		return TransmissionPathOutput{}, err
	}
	generated, err := c.buildTransmissionPathAttempt(ctx, bundle, systemPrompt, userPrompt)
	if err != nil {
		return TransmissionPathOutput{}, err
	}
	if err := generated.ValidateGeneratorOrJudge(); err != nil {
		return TransmissionPathOutput{}, err
	}

	challengeSystemPrompt, err := c.prompts.buildTransmissionPathChallengeInstruction()
	if err != nil {
		return TransmissionPathOutput{}, err
	}
	challengeUserPrompt, err := c.prompts.buildTransmissionPathChallengePrompt(bundle, driverTarget, generated)
	if err != nil {
		return TransmissionPathOutput{}, err
	}
	challenged, err := c.buildTransmissionPathAttempt(ctx, bundle, challengeSystemPrompt, challengeUserPrompt)
	if err != nil {
		return TransmissionPathOutput{}, err
	}
	if err := challenged.ValidateChallenge(); err != nil {
		return TransmissionPathOutput{}, err
	}

	judgeSystemPrompt, err := c.prompts.buildTransmissionPathJudgeInstruction()
	if err != nil {
		return TransmissionPathOutput{}, err
	}
	judgeUserPrompt, err := c.prompts.buildTransmissionPathJudgePrompt(bundle, driverTarget, generated, challenged)
	if err != nil {
		return TransmissionPathOutput{}, err
	}
	final, err := c.buildTransmissionPathAttempt(ctx, bundle, judgeSystemPrompt, judgeUserPrompt)
	if err != nil {
		return TransmissionPathOutput{}, err
	}
	if err := final.ValidateGeneratorOrJudge(); err != nil {
		return TransmissionPathOutput{}, err
	}
	return final, nil
}

func (c *Client) compileEvidenceExplanation(ctx context.Context, bundle Bundle, driverTarget DriverTargetOutput, paths TransmissionPathOutput) (EvidenceExplanationOutput, error) {
	systemPrompt, err := c.prompts.buildEvidenceExplanationGeneratorInstruction()
	if err != nil {
		return EvidenceExplanationOutput{}, err
	}
	userPrompt, err := c.prompts.buildEvidenceExplanationGeneratorPrompt(bundle, driverTarget, paths)
	if err != nil {
		return EvidenceExplanationOutput{}, err
	}
	generated, err := c.buildEvidenceExplanationAttempt(ctx, bundle, systemPrompt, userPrompt)
	if err != nil {
		return EvidenceExplanationOutput{}, err
	}
	if err := generated.ValidateGeneratorOrJudge(); err != nil {
		return EvidenceExplanationOutput{}, err
	}

	challengeSystemPrompt, err := c.prompts.buildEvidenceExplanationChallengeInstruction()
	if err != nil {
		return EvidenceExplanationOutput{}, err
	}
	challengeUserPrompt, err := c.prompts.buildEvidenceExplanationChallengePrompt(bundle, driverTarget, paths, generated)
	if err != nil {
		return EvidenceExplanationOutput{}, err
	}
	challenged, err := c.buildEvidenceExplanationAttempt(ctx, bundle, challengeSystemPrompt, challengeUserPrompt)
	if err != nil {
		return EvidenceExplanationOutput{}, err
	}
	if err := challenged.ValidateChallenge(); err != nil {
		return EvidenceExplanationOutput{}, err
	}

	judgeSystemPrompt, err := c.prompts.buildEvidenceExplanationJudgeInstruction()
	if err != nil {
		return EvidenceExplanationOutput{}, err
	}
	judgeUserPrompt, err := c.prompts.buildEvidenceExplanationJudgePrompt(bundle, driverTarget, paths, generated, challenged)
	if err != nil {
		return EvidenceExplanationOutput{}, err
	}
	final, err := c.buildEvidenceExplanationAttempt(ctx, bundle, judgeSystemPrompt, judgeUserPrompt)
	if err != nil {
		return EvidenceExplanationOutput{}, err
	}
	if err := final.ValidateGeneratorOrJudge(); err != nil {
		return EvidenceExplanationOutput{}, err
	}
	return final, nil
}

func (c *Client) extractNodesAttempt(ctx context.Context, bundle Bundle, systemPrompt string, userPrompt string, reqs GraphRequirements) (NodeExtractionOutput, error) {
	req, err := BuildQwen36ProviderRequest(c.model, bundle, systemPrompt, userPrompt)
	if err != nil {
		return NodeExtractionOutput{}, err
	}
	resp, err := c.runtime.Call(ctx, req)
	if err != nil {
		return NodeExtractionOutput{}, err
	}
	out, err := ParseNodeExtractionOutput(resp.Text)
	if err != nil {
		return NodeExtractionOutput{}, err
	}
	applyBundleTimingFallbacks(bundle, &out.Graph)
	if err := out.ValidateWithThresholds(reqs.MinNodes); err != nil {
		return NodeExtractionOutput{}, err
	}
	return out, nil
}

func (c *Client) buildFullGraphAttempt(ctx context.Context, bundle Bundle, systemPrompt string, userPrompt string, reqs GraphRequirements, nodes []GraphNode) (FullGraphOutput, error) {
	req, err := BuildQwen36ProviderRequest(c.model, bundle, systemPrompt, userPrompt)
	if err != nil {
		return FullGraphOutput{}, err
	}
	resp, err := c.runtime.Call(ctx, req)
	if err != nil {
		return FullGraphOutput{}, err
	}
	nodeIDs, err := validateGraphNodes(nodes)
	if err != nil {
		return FullGraphOutput{}, err
	}
	nodeKinds := graphNodeKinds(nodes)
	out, err := ParseFullGraphOutput(resp.Text, nodeIDs, nodeKinds)
	if err != nil {
		return FullGraphOutput{}, err
	}
	if err := out.ValidateWithThresholds(reqs.MinEdges, nodeIDs, nodeKinds); err != nil {
		return FullGraphOutput{}, err
	}
	return out, nil
}

func (c *Client) buildThesisAttempt(ctx context.Context, bundle Bundle, systemPrompt string, userPrompt string) (ThesisOutput, error) {
	req, err := BuildQwen36ProviderRequest(c.model, bundle, systemPrompt, userPrompt)
	if err != nil {
		return ThesisOutput{}, err
	}
	resp, err := c.runtime.Call(ctx, req)
	if err != nil {
		return ThesisOutput{}, err
	}
	out, err := ParseThesisOutput(resp.Text)
	if err != nil {
		return ThesisOutput{}, err
	}
	return out, nil
}

func (c *Client) buildDriverTargetAttempt(ctx context.Context, bundle Bundle, systemPrompt string, userPrompt string) (DriverTargetOutput, error) {
	req, err := BuildQwen36ProviderRequest(c.model, bundle, systemPrompt, userPrompt)
	if err != nil {
		return DriverTargetOutput{}, err
	}
	resp, err := c.runtime.Call(ctx, req)
	if err != nil {
		return DriverTargetOutput{}, err
	}
	return ParseDriverTargetOutput(resp.Text)
}

func (c *Client) buildTransmissionPathAttempt(ctx context.Context, bundle Bundle, systemPrompt string, userPrompt string) (TransmissionPathOutput, error) {
	req, err := BuildQwen36ProviderRequest(c.model, bundle, systemPrompt, userPrompt)
	if err != nil {
		return TransmissionPathOutput{}, err
	}
	resp, err := c.runtime.Call(ctx, req)
	if err != nil {
		return TransmissionPathOutput{}, err
	}
	return ParseTransmissionPathOutput(resp.Text)
}

func (c *Client) buildEvidenceExplanationAttempt(ctx context.Context, bundle Bundle, systemPrompt string, userPrompt string) (EvidenceExplanationOutput, error) {
	req, err := BuildQwen36ProviderRequest(c.model, bundle, systemPrompt, userPrompt)
	if err != nil {
		return EvidenceExplanationOutput{}, err
	}
	resp, err := c.runtime.Call(ctx, req)
	if err != nil {
		return EvidenceExplanationOutput{}, err
	}
	return ParseEvidenceExplanationOutput(resp.Text)
}

func mergeDirectCompileOutputs(bundle Bundle, driverTarget DriverTargetOutput, paths TransmissionPathOutput, aux EvidenceExplanationOutput) Output {
	topics := aux.Topics
	if len(topics) == 0 {
		topics = paths.Topics
	}
	if len(topics) == 0 {
		topics = driverTarget.Topics
	}
	confidence := strings.TrimSpace(aux.Confidence)
	if confidence == "" {
		confidence = strings.TrimSpace(paths.Confidence)
	}
	if confidence == "" {
		confidence = strings.TrimSpace(driverTarget.Confidence)
	}
	details := aux.Details
	if details.IsEmpty() {
		details = paths.Details
	}
	if details.IsEmpty() {
		details = driverTarget.Details
	}
	graph := buildCompatibilityGraph(bundle, driverTarget, paths, aux)
	return Output{
		Summary:           summarizeDirectCompileOutput(driverTarget, paths),
		Drivers:           driverTarget.Drivers,
		Targets:           driverTarget.Targets,
		TransmissionPaths: paths.TransmissionPaths,
		EvidenceNodes:     aux.EvidenceNodes,
		ExplanationNodes:  aux.ExplanationNodes,
		Graph:             graph,
		Details:           details,
		Topics:            topics,
		Confidence:        confidence,
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
		now = time.Now().UTC()
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

	targetNodeIDs := make([]string, 0, len(driverTarget.Targets))
	for _, target := range driverTarget.Targets {
		targetNodeIDs = append(targetNodeIDs, addNode(NodeConclusion, target))
	}
	primaryTargetID := ""
	if len(targetNodeIDs) > 0 {
		primaryTargetID = targetNodeIDs[0]
	}

	for _, driver := range driverTarget.Drivers {
		addNode(NodeMechanism, driver)
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

	applyBundleTimingFallbacks(bundle, &graph)
	return graph
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

func firstConfiguredValue(projectRoot string, keys ...string) string {
	for _, key := range keys {
		if value, ok := config.Get(projectRoot, key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
