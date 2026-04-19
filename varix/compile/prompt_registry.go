package compile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

type promptRegistry struct {
	promptsDir string
}

func newPromptRegistry(promptsDir string) *promptRegistry {
	trimmed := strings.TrimSpace(promptsDir)
	if trimmed == "" {
		trimmed = resolvePromptsDir()
	}
	return &promptRegistry{promptsDir: trimmed}
}

func (r *promptRegistry) buildInstruction(req GraphRequirements) (string, error) {
	return r.buildThesisInstruction(req)
}

func (r *promptRegistry) buildUnifiedGeneratorInstruction() (string, error) {
	return r.render("compile/unified_generator_system.tmpl", nil)
}

func (r *promptRegistry) buildUnifiedGeneratorPrompt(bundle Bundle) (string, error) {
	payloadJSON, err := marshalCompilePayload(bundle)
	if err != nil {
		return "", err
	}
	return r.render("compile/unified_generator_user.tmpl", map[string]any{
		"PayloadJSON": payloadJSON,
	})
}

func (r *promptRegistry) buildUnifiedChallengeInstruction() (string, error) {
	return r.render("compile/unified_challenge_system.tmpl", nil)
}

func (r *promptRegistry) buildUnifiedChallengePrompt(bundle Bundle, generated UnifiedCompileOutput) (string, error) {
	payloadJSON, err := marshalCompilePayload(bundle)
	if err != nil {
		return "", err
	}
	generatedJSON, err := marshalJSON(generated)
	if err != nil {
		return "", err
	}
	return r.render("compile/unified_challenge_user.tmpl", map[string]any{
		"PayloadJSON":   payloadJSON,
		"GeneratedJSON": generatedJSON,
	})
}

func (r *promptRegistry) buildUnifiedJudgeInstruction() (string, error) {
	return r.render("compile/unified_judge_system.tmpl", nil)
}

func (r *promptRegistry) buildUnifiedJudgePrompt(bundle Bundle, generated UnifiedCompileOutput, challenged UnifiedCompileOutput) (string, error) {
	payloadJSON, err := marshalCompilePayload(bundle)
	if err != nil {
		return "", err
	}
	generatedJSON, err := marshalJSON(generated)
	if err != nil {
		return "", err
	}
	challengedJSON, err := marshalJSON(challenged)
	if err != nil {
		return "", err
	}
	return r.render("compile/unified_judge_user.tmpl", map[string]any{
		"PayloadJSON":    payloadJSON,
		"GeneratedJSON":  generatedJSON,
		"ChallengedJSON": challengedJSON,
	})
}

func (r *promptRegistry) buildThesisInstruction(req GraphRequirements) (string, error) {
	return r.render("compile/system.tmpl", map[string]any{
		"MinNodes": req.MinNodes,
		"MinEdges": req.MinEdges,
	})
}

func (r *promptRegistry) buildPrompt(bundle Bundle) (string, error) {
	return r.buildThesisPrompt(bundle, ReasoningGraph{})
}

func (r *promptRegistry) buildThesisPrompt(bundle Bundle, projection ReasoningGraph) (string, error) {
	payloadJSON, err := marshalCompilePayload(bundle)
	if err != nil {
		return "", err
	}
	projectionJSON, err := marshalReasoningGraph(projection)
	if err != nil {
		return "", err
	}
	return r.render("compile/user.tmpl", map[string]any{
		"PayloadJSON":    payloadJSON,
		"ProjectionJSON": projectionJSON,
	})
}

func (r *promptRegistry) buildRetryPrompt(bundle Bundle, req GraphRequirements) (string, error) {
	return r.buildThesisRetryPrompt(bundle, ReasoningGraph{}, req)
}

func (r *promptRegistry) buildThesisRetryPrompt(bundle Bundle, projection ReasoningGraph, req GraphRequirements) (string, error) {
	basePrompt, err := r.buildThesisPrompt(bundle, projection)
	if err != nil {
		return "", err
	}
	suffix, err := r.render("compile/retry_suffix.tmpl", map[string]any{
		"MinNodes": req.MinNodes,
		"MinEdges": req.MinEdges,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(basePrompt + "\n\n" + suffix), nil
}

func (r *promptRegistry) buildGraphInstruction(req GraphRequirements) (string, error) {
	return r.render("compile/graph_system.tmpl", map[string]any{
		"MinEdges": req.MinEdges,
	})
}

func (r *promptRegistry) buildGraphPrompt(bundle Bundle, nodes []GraphNode) (string, error) {
	payloadJSON, err := marshalCompilePayload(bundle)
	if err != nil {
		return "", err
	}
	nodesJSON, err := marshalGraphNodes(nodes)
	if err != nil {
		return "", err
	}
	return r.render("compile/graph_user.tmpl", map[string]any{
		"PayloadJSON": payloadJSON,
		"NodesJSON":   nodesJSON,
	})
}

func (r *promptRegistry) buildGraphRetryPrompt(bundle Bundle, nodes []GraphNode, req GraphRequirements) (string, error) {
	basePrompt, err := r.buildGraphPrompt(bundle, nodes)
	if err != nil {
		return "", err
	}
	suffix, err := r.render("compile/graph_retry_suffix.tmpl", map[string]any{
		"MinEdges": req.MinEdges,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(basePrompt + "\n\n" + suffix), nil
}

func (r *promptRegistry) buildNodeChallengeInstruction(req GraphRequirements) (string, error) {
	return r.render("compile/node_challenge_system.tmpl", map[string]any{
		"MinNodes": req.MinNodes,
	})
}

func (r *promptRegistry) buildNodeChallengePrompt(bundle Bundle, nodes []GraphNode) (string, error) {
	payloadJSON, err := marshalCompilePayload(bundle)
	if err != nil {
		return "", err
	}
	nodesJSON, err := marshalGraphNodes(nodes)
	if err != nil {
		return "", err
	}
	return r.render("compile/node_challenge_user.tmpl", map[string]any{
		"PayloadJSON": payloadJSON,
		"NodesJSON":   nodesJSON,
	})
}

func (r *promptRegistry) buildNodeChallengeRetryPrompt(bundle Bundle, nodes []GraphNode, req GraphRequirements) (string, error) {
	basePrompt, err := r.buildNodeChallengePrompt(bundle, nodes)
	if err != nil {
		return "", err
	}
	suffix, err := r.render("compile/node_challenge_retry_suffix.tmpl", map[string]any{
		"MinNodes": req.MinNodes,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(basePrompt + "\n\n" + suffix), nil
}

func (r *promptRegistry) buildNodeInstruction(req GraphRequirements) (string, error) {
	return r.render("compile/node_system.tmpl", map[string]any{
		"MinNodes": req.MinNodes,
	})
}

func (r *promptRegistry) buildNodePrompt(bundle Bundle) (string, error) {
	payloadJSON, err := marshalCompilePayload(bundle)
	if err != nil {
		return "", err
	}
	return r.render("compile/node_user.tmpl", map[string]any{
		"PayloadJSON": payloadJSON,
	})
}

func (r *promptRegistry) buildNodeRetryPrompt(bundle Bundle, req GraphRequirements) (string, error) {
	basePrompt, err := r.buildNodePrompt(bundle)
	if err != nil {
		return "", err
	}
	suffix, err := r.render("compile/node_retry_suffix.tmpl", map[string]any{
		"MinNodes": req.MinNodes,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(basePrompt + "\n\n" + suffix), nil
}

func (r *promptRegistry) buildEdgeChallengeInstruction(req GraphRequirements) (string, error) {
	return r.render("compile/edge_challenge_system.tmpl", map[string]any{
		"MinEdges": req.MinEdges,
	})
}

func (r *promptRegistry) buildEdgeChallengePrompt(bundle Bundle, nodes []GraphNode, edges []GraphEdge) (string, error) {
	payloadJSON, err := marshalCompilePayload(bundle)
	if err != nil {
		return "", err
	}
	nodesJSON, err := marshalGraphNodes(nodes)
	if err != nil {
		return "", err
	}
	edgesJSON, err := marshalGraphEdges(edges)
	if err != nil {
		return "", err
	}
	return r.render("compile/edge_challenge_user.tmpl", map[string]any{
		"PayloadJSON": payloadJSON,
		"NodesJSON":   nodesJSON,
		"EdgesJSON":   edgesJSON,
	})
}

func (r *promptRegistry) buildEdgeChallengeRetryPrompt(bundle Bundle, nodes []GraphNode, edges []GraphEdge, req GraphRequirements) (string, error) {
	basePrompt, err := r.buildEdgeChallengePrompt(bundle, nodes, edges)
	if err != nil {
		return "", err
	}
	suffix, err := r.render("compile/edge_challenge_retry_suffix.tmpl", map[string]any{
		"MinEdges": req.MinEdges,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(basePrompt + "\n\n" + suffix), nil
}

func (r *promptRegistry) verifierInstruction(name string) (string, error) {
	return r.render(filepath.ToSlash(filepath.Join("compile", "verifier", name+".tmpl")), nil)
}

func (r *promptRegistry) render(relativePath string, data any) (string, error) {
	if r == nil || strings.TrimSpace(r.promptsDir) == "" {
		return "", fmt.Errorf("prompt registry has no prompts directory")
	}
	body, err := r.loadPromptBody(relativePath)
	if err != nil {
		return "", err
	}
	tmpl, err := template.New(relativePath).Option("missingkey=error").Parse(string(body))
	if err != nil {
		return "", fmt.Errorf("parse prompt %s: %w", relativePath, err)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("render prompt %s: %w", relativePath, err)
	}
	return strings.TrimSpace(rendered.String()), nil
}

func (r *promptRegistry) loadPromptBody(relativePath string) ([]byte, error) {
	candidate := filepath.Join(r.promptsDir, filepath.FromSlash(relativePath))
	body, err := os.ReadFile(candidate)
	if err == nil {
		return body, nil
	}
	defaultDir := resolvePromptsDir()
	if strings.TrimSpace(defaultDir) != "" && !samePath(defaultDir, r.promptsDir) {
		fallback := filepath.Join(defaultDir, filepath.FromSlash(relativePath))
		body, fallbackErr := os.ReadFile(fallback)
		if fallbackErr == nil {
			return body, nil
		}
	}
	return nil, fmt.Errorf("load prompt %s: %w", relativePath, err)
}

func samePath(a, b string) bool {
	a = filepath.Clean(strings.TrimSpace(a))
	b = filepath.Clean(strings.TrimSpace(b))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func marshalCompilePayload(bundle Bundle) (string, error) {
	payload := map[string]any{
		"unit_id":           bundle.UnitID,
		"source":            bundle.Source,
		"external_id":       bundle.ExternalID,
		"root_external_id":  bundle.RootExternalID,
		"author_name":       bundle.AuthorName,
		"author_id":         bundle.AuthorID,
		"url":               bundle.URL,
		"posted_at":         bundle.PostedAt,
		"quote_count":       len(bundle.Quotes),
		"reference_count":   len(bundle.References),
		"thread_count":      len(bundle.ThreadSegments),
		"attachment_count":  len(bundle.Attachments),
		"local_image_paths": bundle.LocalImagePaths,
		"text_context":      bundle.TextContext(),
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func marshalJSON(value any) (string, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func marshalGraphNodes(nodes []GraphNode) (string, error) {
	if len(nodes) == 0 {
		return "[]", nil
	}
	encoded, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func marshalReasoningGraph(graph ReasoningGraph) (string, error) {
	encoded, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func marshalGraphEdges(edges []GraphEdge) (string, error) {
	if len(edges) == 0 {
		return "[]", nil
	}
	encoded, err := json.MarshalIndent(edges, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func resolvePromptsDir() string {
	candidates := make([]string, 0, 3)
	if root := strings.TrimSpace(os.Getenv("VARIX_ROOT")); root != "" {
		candidates = append(candidates, filepath.Join(root, "prompts"))
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "prompts")))
	}
	if wd, err := os.Getwd(); err == nil {
		for dir := filepath.Clean(wd); ; dir = filepath.Dir(dir) {
			candidates = append(candidates, filepath.Join(dir, "prompts"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
	}
	return filepath.Join("prompts")
}
