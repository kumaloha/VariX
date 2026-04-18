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
	reqs := InferGraphRequirements(bundle)
	nodeSystemPrompt, err := c.prompts.buildNodeInstruction(reqs)
	if err != nil {
		return Record{}, err
	}
	nodeUserPrompt, err := c.prompts.buildNodePrompt(bundle)
	if err != nil {
		return Record{}, err
	}
	nodeOutput, err := c.extractNodesAttempt(ctx, bundle, nodeSystemPrompt, nodeUserPrompt, reqs)
	if err != nil {
		retryPrompt, retryErr := c.prompts.buildNodeRetryPrompt(bundle, reqs)
		if retryErr != nil {
			return Record{}, retryErr
		}
		nodeOutput, err = c.extractNodesAttempt(
			ctx,
			bundle,
			nodeSystemPrompt,
			retryPrompt,
			reqs,
		)
		if err != nil {
			return Record{}, err
		}
	}
	nodeChallengeSystemPrompt, err := c.prompts.buildNodeChallengeInstruction(reqs)
	if err != nil {
		return Record{}, err
	}
	nodeChallengeUserPrompt, err := c.prompts.buildNodeChallengePrompt(bundle, nodeOutput.Graph.Nodes)
	if err != nil {
		return Record{}, err
	}
	nodeChallengeOutput, err := c.extractNodesAttempt(ctx, bundle, nodeChallengeSystemPrompt, nodeChallengeUserPrompt, GraphRequirements{})
	if err != nil {
		retryPrompt, retryErr := c.prompts.buildNodeChallengeRetryPrompt(bundle, nodeOutput.Graph.Nodes, reqs)
		if retryErr != nil {
			return Record{}, retryErr
		}
		nodeChallengeOutput, err = c.extractNodesAttempt(ctx, bundle, nodeChallengeSystemPrompt, retryPrompt, GraphRequirements{})
		if err != nil {
			nodeChallengeOutput = NodeExtractionOutput{}
		}
	}
	nodeOutput = mergeNodeOutputs(nodeOutput, nodeChallengeOutput)
	fullGraphSystemPrompt, err := c.prompts.buildGraphInstruction(reqs)
	if err != nil {
		return Record{}, err
	}
	fullGraphUserPrompt, err := c.prompts.buildGraphPrompt(bundle, nodeOutput.Graph.Nodes)
	if err != nil {
		return Record{}, err
	}
	fullGraphOutput, err := c.buildFullGraphAttempt(ctx, bundle, fullGraphSystemPrompt, fullGraphUserPrompt, reqs, nodeOutput.Graph.Nodes)
	if err != nil {
		retryPrompt, retryErr := c.prompts.buildGraphRetryPrompt(bundle, nodeOutput.Graph.Nodes, reqs)
		if retryErr != nil {
			return Record{}, retryErr
		}
		fullGraphOutput, err = c.buildFullGraphAttempt(ctx, bundle, fullGraphSystemPrompt, retryPrompt, reqs, nodeOutput.Graph.Nodes)
		if err != nil {
			return Record{}, err
		}
	}
	edgeChallengeSystemPrompt, err := c.prompts.buildEdgeChallengeInstruction(reqs)
	if err != nil {
		return Record{}, err
	}
	edgeChallengeUserPrompt, err := c.prompts.buildEdgeChallengePrompt(bundle, nodeOutput.Graph.Nodes, fullGraphOutput.Graph.Edges)
	if err != nil {
		return Record{}, err
	}
	edgeChallengeOutput, err := c.buildFullGraphAttempt(ctx, bundle, edgeChallengeSystemPrompt, edgeChallengeUserPrompt, GraphRequirements{}, nodeOutput.Graph.Nodes)
	if err != nil {
		retryPrompt, retryErr := c.prompts.buildEdgeChallengeRetryPrompt(bundle, nodeOutput.Graph.Nodes, fullGraphOutput.Graph.Edges, reqs)
		if retryErr != nil {
			return Record{}, retryErr
		}
		edgeChallengeOutput, err = c.buildFullGraphAttempt(ctx, bundle, edgeChallengeSystemPrompt, retryPrompt, GraphRequirements{}, nodeOutput.Graph.Nodes)
		if err != nil {
			edgeChallengeOutput = FullGraphOutput{}
		}
	}
	fullGraphOutput = mergeFullGraphOutputs(fullGraphOutput, edgeChallengeOutput)
	causalProjection := buildCausalProjection(nodeOutput.Graph.Nodes, fullGraphOutput.Graph.Edges)
	thesisSystemPrompt, err := c.prompts.buildThesisInstruction(reqs)
	if err != nil {
		return Record{}, err
	}
	thesisUserPrompt, err := c.prompts.buildThesisPrompt(bundle, causalProjection)
	if err != nil {
		return Record{}, err
	}
	thesisOutput, err := c.buildThesisAttempt(ctx, bundle, thesisSystemPrompt, thesisUserPrompt)
	if err != nil {
		retryPrompt, retryErr := c.prompts.buildThesisRetryPrompt(bundle, causalProjection, reqs)
		if retryErr != nil {
			return Record{}, retryErr
		}
		thesisOutput, err = c.buildThesisAttempt(ctx, bundle, thesisSystemPrompt, retryPrompt)
		if err != nil {
			return Record{}, err
		}
	}
	output := mergeCompileOutputs(nodeOutput, fullGraphOutput, thesisOutput)
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
		case NodeFact, NodeImplicitCondition:
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
