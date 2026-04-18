package compile

import (
	"context"
	"errors"
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
	output, err := c.compileDirect(ctx, bundle)
	if err != nil {
		var legacyStageErr legacyNodeStageError
		switch {
		case errors.As(err, &legacyStageErr):
			output, err = c.compileLegacyFromInitialNodeOutput(ctx, bundle, legacyStageErr.nodeOutput)
		case shouldFallbackToLegacyCompile(err):
			output, err = c.compileLegacy(ctx, bundle)
		}
	}
	if err != nil {
		return Record{}, err
	}
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

type legacyNodeStageError struct {
	nodeOutput NodeExtractionOutput
}

func (e legacyNodeStageError) Error() string {
	return "direct pipeline received legacy node stage output"
}

func shouldFallbackToLegacyCompile(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "drivers must not be empty") ||
		strings.Contains(msg, "targets must not be empty") ||
		strings.Contains(msg, "load prompt compile/driver_target_")
}

func (c *Client) compileDirect(ctx context.Context, bundle Bundle) (Output, error) {
	driverTargetOutput, err := c.compileDriverTarget(ctx, bundle)
	if err != nil {
		return Output{}, err
	}
	transmissionPathOutput, err := c.compileTransmissionPaths(ctx, bundle, driverTargetOutput)
	if err != nil {
		return Output{}, err
	}
	evidenceExplanationOutput, err := c.compileEvidenceExplanation(ctx, bundle, driverTargetOutput, transmissionPathOutput)
	if err != nil {
		return Output{}, err
	}
	return mergeDirectCompileOutputs(bundle, driverTargetOutput, transmissionPathOutput, evidenceExplanationOutput), nil
}

func (c *Client) compileLegacy(ctx context.Context, bundle Bundle) (Output, error) {
	reqs := InferGraphRequirements(bundle)
	nodeSystemPrompt, err := c.prompts.buildNodeInstruction(reqs)
	if err != nil {
		return Output{}, err
	}
	nodeUserPrompt, err := c.prompts.buildNodePrompt(bundle)
	if err != nil {
		return Output{}, err
	}
	nodeOutput, err := c.extractNodesAttempt(ctx, bundle, nodeSystemPrompt, nodeUserPrompt, reqs)
	if err != nil {
		return Output{}, err
	}
	return c.compileLegacyFromInitialNodeOutput(ctx, bundle, nodeOutput)
}

func (c *Client) compileLegacyFromInitialNodeOutput(ctx context.Context, bundle Bundle, nodeOutput NodeExtractionOutput) (Output, error) {
	reqs := InferGraphRequirements(bundle)
	nodeSystemPrompt, err := c.prompts.buildNodeInstruction(reqs)
	if err != nil {
		return Output{}, err
	}
	if err := nodeOutput.ValidateWithThresholds(reqs.MinNodes); err != nil {
		retryPrompt, retryErr := c.prompts.buildNodeRetryPrompt(bundle, reqs)
		if retryErr != nil {
			return Output{}, retryErr
		}
		nodeOutput, err = c.extractNodesAttempt(ctx, bundle, nodeSystemPrompt, retryPrompt, reqs)
		if err != nil {
			return Output{}, err
		}
	}

	nodeChallengeSystemPrompt, err := c.prompts.buildNodeChallengeInstruction(reqs)
	if err != nil {
		return Output{}, err
	}
	nodeChallengeUserPrompt, err := c.prompts.buildNodeChallengePrompt(bundle, nodeOutput.Graph.Nodes)
	if err != nil {
		return Output{}, err
	}
	nodeChallengeOutput, err := c.extractNodesAttempt(ctx, bundle, nodeChallengeSystemPrompt, nodeChallengeUserPrompt, GraphRequirements{})
	if err != nil {
		retryPrompt, retryErr := c.prompts.buildNodeChallengeRetryPrompt(bundle, nodeOutput.Graph.Nodes, reqs)
		if retryErr != nil {
			return Output{}, retryErr
		}
		nodeChallengeOutput, err = c.extractNodesAttempt(ctx, bundle, nodeChallengeSystemPrompt, retryPrompt, GraphRequirements{})
		if err != nil {
			nodeChallengeOutput = NodeExtractionOutput{}
		}
	}
	nodeOutput = mergeNodeOutputs(nodeOutput, nodeChallengeOutput)

	fullGraphSystemPrompt, err := c.prompts.buildGraphInstruction(reqs)
	if err != nil {
		return Output{}, err
	}
	fullGraphUserPrompt, err := c.prompts.buildGraphPrompt(bundle, nodeOutput.Graph.Nodes)
	if err != nil {
		return Output{}, err
	}
	fullGraphOutput, err := c.buildFullGraphAttempt(ctx, bundle, fullGraphSystemPrompt, fullGraphUserPrompt, reqs, nodeOutput.Graph.Nodes)
	if err != nil {
		retryPrompt, retryErr := c.prompts.buildGraphRetryPrompt(bundle, nodeOutput.Graph.Nodes, reqs)
		if retryErr != nil {
			return Output{}, retryErr
		}
		fullGraphOutput, err = c.buildFullGraphAttempt(ctx, bundle, fullGraphSystemPrompt, retryPrompt, reqs, nodeOutput.Graph.Nodes)
		if err != nil {
			return Output{}, err
		}
	}

	edgeChallengeSystemPrompt, err := c.prompts.buildEdgeChallengeInstruction(reqs)
	if err != nil {
		return Output{}, err
	}
	edgeChallengeUserPrompt, err := c.prompts.buildEdgeChallengePrompt(bundle, nodeOutput.Graph.Nodes, fullGraphOutput.Graph.Edges)
	if err != nil {
		return Output{}, err
	}
	edgeChallengeOutput, err := c.buildFullGraphAttempt(ctx, bundle, edgeChallengeSystemPrompt, edgeChallengeUserPrompt, GraphRequirements{}, nodeOutput.Graph.Nodes)
	if err != nil {
		retryPrompt, retryErr := c.prompts.buildEdgeChallengeRetryPrompt(bundle, nodeOutput.Graph.Nodes, fullGraphOutput.Graph.Edges, reqs)
		if retryErr != nil {
			return Output{}, retryErr
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
		return Output{}, err
	}
	thesisUserPrompt, err := c.prompts.buildThesisPrompt(bundle, causalProjection)
	if err != nil {
		return Output{}, err
	}
	thesisOutput, err := c.buildThesisAttempt(ctx, bundle, thesisSystemPrompt, thesisUserPrompt)
	if err != nil {
		retryPrompt, retryErr := c.prompts.buildThesisRetryPrompt(bundle, causalProjection, reqs)
		if retryErr != nil {
			return Output{}, retryErr
		}
		thesisOutput, err = c.buildThesisAttempt(ctx, bundle, thesisSystemPrompt, retryPrompt)
		if err != nil {
			return Output{}, err
		}
	}

	return mergeCompileOutputs(nodeOutput, fullGraphOutput, thesisOutput), nil
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
	challenged = mergeDriverTargetOutputs(DriverTargetOutput{}, challenged)

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
	final = mergeDriverTargetOutputs(generated, challenged, final)
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
	challenged = mergeTransmissionPathOutputs(TransmissionPathOutput{}, challenged)

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
	final = mergeTransmissionPathOutputs(generated, challenged, final)
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
	challenged = mergeEvidenceExplanationOutputs(EvidenceExplanationOutput{}, challenged)

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
	final = mergeEvidenceExplanationOutputs(generated, challenged, final)
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
	out, err := ParseDriverTargetOutput(resp.Text)
	if err != nil {
		return DriverTargetOutput{}, err
	}
	if len(out.Drivers) == 0 || len(out.Targets) == 0 {
		if legacy, legacyErr := ParseOutput(resp.Text); legacyErr == nil {
			out = mergeDriverTargetOutputs(out, deriveDriverTargetOutputFromLegacy(legacy))
		}
	}
	if len(out.Drivers) == 0 && len(out.Targets) == 0 && looksLikeLegacyGraphPayload(resp.Text) {
		legacyNodeOutput, legacyErr := ParseNodeExtractionOutput(resp.Text)
		if legacyErr == nil && len(legacyNodeOutput.Graph.Nodes) > 0 {
			applyBundleTimingFallbacks(bundle, &legacyNodeOutput.Graph)
			return DriverTargetOutput{}, legacyNodeStageError{nodeOutput: legacyNodeOutput}
		}
	}
	return out, nil
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
	out, err := ParseTransmissionPathOutput(resp.Text)
	if err != nil {
		return TransmissionPathOutput{}, err
	}
	if len(out.TransmissionPaths) == 0 && looksLikeLegacyGraphPayload(resp.Text) {
		if legacy, legacyErr := ParseOutput(resp.Text); legacyErr == nil {
			out = deriveTransmissionPathOutputFromLegacy(legacy)
		}
	}
	return out, nil
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
	out, err := ParseEvidenceExplanationOutput(resp.Text)
	if err != nil {
		return EvidenceExplanationOutput{}, err
	}
	if len(out.EvidenceNodes) == 0 && len(out.ExplanationNodes) == 0 && looksLikeLegacyGraphPayload(resp.Text) {
		if legacy, legacyErr := ParseOutput(resp.Text); legacyErr == nil {
			out = deriveEvidenceExplanationOutputFromLegacy(legacy)
		}
	}
	return out, nil
}

func looksLikeLegacyGraphPayload(raw string) bool {
	payload, err := parseCompilePayload(raw)
	if err != nil {
		return false
	}
	_, hasGraph := payload["graph"]
	_, hasSummary := payload["summary"]
	return hasGraph || hasSummary
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

func mergeDriverTargetOutputs(outputs ...DriverTargetOutput) DriverTargetOutput {
	var merged DriverTargetOutput
	for _, out := range outputs {
		merged.Drivers = appendUniqueNormalized(merged.Drivers, out.Drivers...)
		merged.Targets = appendUniqueNormalized(merged.Targets, out.Targets...)
		if merged.Details.IsEmpty() && !out.Details.IsEmpty() {
			merged.Details = out.Details
		}
		if len(merged.Topics) == 0 && len(out.Topics) > 0 {
			merged.Topics = append([]string(nil), out.Topics...)
		}
		if strings.TrimSpace(merged.Confidence) == "" && strings.TrimSpace(out.Confidence) != "" {
			merged.Confidence = out.Confidence
		}
	}
	return merged
}

func mergeTransmissionPathOutputs(outputs ...TransmissionPathOutput) TransmissionPathOutput {
	var merged TransmissionPathOutput
	seen := map[string]struct{}{}
	for _, out := range outputs {
		for _, path := range out.TransmissionPaths {
			key := normalizedPathKey(path)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged.TransmissionPaths = append(merged.TransmissionPaths, path)
		}
		if merged.Details.IsEmpty() && !out.Details.IsEmpty() {
			merged.Details = out.Details
		}
		if len(merged.Topics) == 0 && len(out.Topics) > 0 {
			merged.Topics = append([]string(nil), out.Topics...)
		}
		if strings.TrimSpace(merged.Confidence) == "" && strings.TrimSpace(out.Confidence) != "" {
			merged.Confidence = out.Confidence
		}
	}
	return merged
}

func mergeEvidenceExplanationOutputs(outputs ...EvidenceExplanationOutput) EvidenceExplanationOutput {
	var merged EvidenceExplanationOutput
	for _, out := range outputs {
		merged.EvidenceNodes = appendUniqueNormalized(merged.EvidenceNodes, out.EvidenceNodes...)
		merged.ExplanationNodes = appendUniqueNormalized(merged.ExplanationNodes, out.ExplanationNodes...)
		if merged.Details.IsEmpty() && !out.Details.IsEmpty() {
			merged.Details = out.Details
		}
		if len(merged.Topics) == 0 && len(out.Topics) > 0 {
			merged.Topics = append([]string(nil), out.Topics...)
		}
		if strings.TrimSpace(merged.Confidence) == "" && strings.TrimSpace(out.Confidence) != "" {
			merged.Confidence = out.Confidence
		}
	}
	return merged
}

func deriveDriverTargetOutputFromLegacy(out Output) DriverTargetOutput {
	derived := DriverTargetOutput{
		Drivers:    append([]string(nil), out.Drivers...),
		Targets:    append([]string(nil), out.Targets...),
		Details:    out.Details,
		Topics:     out.Topics,
		Confidence: out.Confidence,
	}
	if len(derived.Drivers) == 0 || len(derived.Targets) == 0 {
		for _, node := range out.Graph.Nodes {
			switch node.Kind {
			case NodeMechanism:
				derived.Drivers = appendUniqueNormalized(derived.Drivers, node.Text)
			case NodeConclusion, NodePrediction:
				derived.Targets = appendUniqueNormalized(derived.Targets, node.Text)
			}
		}
	}
	if len(derived.Drivers) == 0 {
		for _, node := range out.Graph.Nodes {
			if node.Kind == NodeFact {
				derived.Drivers = appendUniqueNormalized(derived.Drivers, node.Text)
				break
			}
		}
	}
	if len(derived.Targets) == 0 && strings.TrimSpace(out.Summary) != "" {
		derived.Targets = appendUniqueNormalized(derived.Targets, out.Summary)
	}
	return derived
}

func deriveTransmissionPathOutputFromLegacy(out Output) TransmissionPathOutput {
	driverTarget := deriveDriverTargetOutputFromLegacy(out)
	derived := TransmissionPathOutput{
		Details:    out.Details,
		Topics:     out.Topics,
		Confidence: out.Confidence,
	}
	for _, edge := range out.Graph.Edges {
		if edge.Kind != EdgePositive {
			continue
		}
		from := findNodeText(out.Graph.Nodes, edge.From)
		to := findNodeText(out.Graph.Nodes, edge.To)
		if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
			continue
		}
		derived.TransmissionPaths = append(derived.TransmissionPaths, TransmissionPath{
			Driver: firstNormalizedOrFallback(driverTarget.Drivers, from),
			Target: to,
			Steps:  []string{from},
		})
	}
	if len(derived.TransmissionPaths) == 0 && len(driverTarget.Drivers) > 0 && len(driverTarget.Targets) > 0 {
		derived.TransmissionPaths = append(derived.TransmissionPaths, TransmissionPath{
			Driver: driverTarget.Drivers[0],
			Target: driverTarget.Targets[0],
			Steps:  []string{driverTarget.Drivers[0]},
		})
	}
	return derived
}

func deriveEvidenceExplanationOutputFromLegacy(out Output) EvidenceExplanationOutput {
	driverTarget := deriveDriverTargetOutputFromLegacy(out)
	derived := EvidenceExplanationOutput{
		EvidenceNodes:    append([]string(nil), out.EvidenceNodes...),
		ExplanationNodes: append([]string(nil), out.ExplanationNodes...),
		Details:          out.Details,
		Topics:           out.Topics,
		Confidence:       out.Confidence,
	}
	targets := make(map[string]struct{}, len(driverTarget.Targets))
	for _, target := range driverTarget.Targets {
		targets[normalizeLooseText(target)] = struct{}{}
	}
	for _, node := range out.Graph.Nodes {
		switch node.Kind {
		case NodeFact:
			derived.EvidenceNodes = appendUniqueNormalized(derived.EvidenceNodes, node.Text)
		case NodeConclusion:
			if _, ok := targets[normalizeLooseText(node.Text)]; !ok {
				derived.ExplanationNodes = appendUniqueNormalized(derived.ExplanationNodes, node.Text)
			}
		}
	}
	return derived
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

func normalizedPathKey(path TransmissionPath) string {
	return normalizeLooseText(path.Driver) + "->" + normalizeLooseText(path.Target) + "::" + normalizeLooseText(strings.Join(path.Steps, "|"))
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

func firstConfiguredValue(projectRoot string, keys ...string) string {
	for _, key := range keys {
		if value, ok := config.Get(projectRoot, key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
