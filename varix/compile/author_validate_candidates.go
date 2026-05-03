package compile

import (
	"fmt"
	"strings"
)

func collectAuthorClaimCandidates(out Output) []authorClaimCandidate {
	candidates := make([]authorClaimCandidate, 0)
	seen := map[string]struct{}{}
	provenance := authorClaimProvenanceByKey(out.Details.Items)
	add := func(kind, text, branch string, item map[string]any) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if item == nil {
			item = provenance[authorClaimProvenanceKey(kind, text)]
		}
		keyBranch := strings.TrimSpace(branch)
		if kind == "render_node" {
			keyBranch = ""
		}
		key := kind + "\x00" + keyBranch + "\x00" + text
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		id := fmt.Sprintf("claim-%03d", len(candidates)+1)
		candidates = append(candidates, authorClaimCandidate{
			ClaimID: id,
			Kind:    kind,
			Text:    text,
			Branch:  strings.TrimSpace(branch),
		})
		applyAuthorClaimProvenance(&candidates[len(candidates)-1], item)
	}
	addRenderNodes := func(branch string, values ...[]string) {
		for _, group := range values {
			for _, value := range group {
				add("render_node", value, branch, nil)
			}
		}
	}
	addRenderPathNodes := func(branch string, paths []TransmissionPath) {
		for _, path := range paths {
			add("render_node", path.Driver, branch, nil)
			addRenderNodes(branch, path.Steps)
			add("render_node", path.Target, branch, nil)
		}
	}
	addRenderNodes("", out.Drivers, out.Targets)
	addRenderPathNodes("", out.TransmissionPaths)
	for _, value := range out.EvidenceNodes {
		add("proof_point", value, "", nil)
	}
	for _, item := range out.Details.Items {
		kind := hiddenDetailString(item, "kind")
		if !isAuthorClaimDetailKind(kind) {
			continue
		}
		add(kind, hiddenDetailString(item, "text"), hiddenDetailString(item, "branch"), item)
	}
	for _, branch := range out.Branches {
		branchID := firstTrimmed(branch.ID, branch.Thesis)
		addRenderNodes(branchID, branch.Anchors, branch.Drivers, branch.BranchDrivers, branch.Targets)
		addRenderPathNodes(branchID, branch.TransmissionPaths)
	}
	return candidates
}

func authorClaimProvenanceByKey(items []map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(items))
	for _, item := range items {
		kind := hiddenDetailString(item, "kind")
		if !isAuthorClaimDetailKind(kind) {
			continue
		}
		text := hiddenDetailString(item, "text")
		if text == "" {
			continue
		}
		out[authorClaimProvenanceKey(kind, text)] = item
	}
	return out
}

func authorClaimProvenanceKey(kind, text string) string {
	return strings.TrimSpace(kind) + "\x00" + strings.TrimSpace(text)
}

func isAuthorClaimDetailKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "render_node", "proof_point":
		return true
	default:
		return false
	}
}

func applyAuthorClaimProvenance(candidate *authorClaimCandidate, item map[string]any) {
	if candidate == nil || item == nil {
		return
	}
	candidate.SourceText = hiddenDetailString(item, "source_text")
	candidate.SourceQuote = hiddenDetailString(item, "source_quote")
	candidate.Role = hiddenDetailString(item, "role")
	candidate.AttachesTo = hiddenDetailString(item, "attaches_to")
	candidate.Context = hiddenDetailString(item, "context")
}

func hiddenDetailString(item map[string]any, key string) string {
	if item == nil {
		return ""
	}
	value, ok := item[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func collectAuthorInferenceCandidates(out Output) []authorInferenceCandidate {
	candidates := make([]authorInferenceCandidate, 0)
	seen := map[string]struct{}{}
	provenance := authorInferenceProvenanceByKey(out.Details.Items)
	add := func(path TransmissionPath, branch string) {
		from := strings.TrimSpace(path.Driver)
		to := strings.TrimSpace(path.Target)
		if from == "" || to == "" {
			return
		}
		steps := cloneStrings(path.Steps)
		key := authorInferenceProvenanceKey(branch, from, steps, to)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		item := authorInferenceProvenanceForPath(provenance, strings.TrimSpace(branch), from, steps, to)
		id := fmt.Sprintf("inference-%03d", len(candidates)+1)
		candidates = append(candidates, authorInferenceCandidate{
			InferenceID: id,
			From:        from,
			To:          to,
			Steps:       steps,
			Branch:      strings.TrimSpace(branch),
		})
		applyAuthorInferenceProvenance(&candidates[len(candidates)-1], item)
	}
	for _, path := range out.TransmissionPaths {
		add(path, "")
	}
	for _, branch := range out.Branches {
		branchID := firstTrimmed(branch.ID, branch.Thesis)
		for _, path := range branch.TransmissionPaths {
			add(path, branchID)
		}
	}
	return candidates
}

func authorInferenceProvenanceForPath(provenance map[string]map[string]any, branch, from string, steps []string, to string) map[string]any {
	branches := []string{strings.TrimSpace(branch)}
	if strings.TrimSpace(branch) != "" {
		branches = append(branches, "")
	}
	stepVariants := [][]string{cloneStrings(steps)}
	if len(steps) == 1 && normalizeText(steps[0]) == normalizeText(from) {
		stepVariants = append(stepVariants, nil)
	}
	if len(steps) == 0 {
		stepVariants = append(stepVariants, []string{from})
	}
	for _, candidateBranch := range branches {
		for _, candidateSteps := range stepVariants {
			if item := provenance[authorInferenceProvenanceKey(candidateBranch, from, candidateSteps, to)]; item != nil {
				return item
			}
		}
	}
	return nil
}

func authorInferenceProvenanceByKey(items []map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(items))
	for _, item := range items {
		if hiddenDetailString(item, "kind") != "inference_path" {
			continue
		}
		from := hiddenDetailString(item, "from")
		to := hiddenDetailString(item, "to")
		if from == "" || to == "" {
			continue
		}
		steps := hiddenDetailStringSlice(item, "steps")
		branch := hiddenDetailString(item, "branch")
		out[authorInferenceProvenanceKey(branch, from, steps, to)] = item
		out[authorInferenceProvenanceKey("", from, steps, to)] = item
	}
	return out
}

func authorInferenceProvenanceKey(branch, from string, steps []string, to string) string {
	return strings.TrimSpace(branch) + "\x00" + strings.TrimSpace(from) + "\x00" + strings.Join(trimmedStringSlice(steps), "\x00") + "\x00" + strings.TrimSpace(to)
}

func applyAuthorInferenceProvenance(candidate *authorInferenceCandidate, item map[string]any) {
	if candidate == nil || item == nil {
		return
	}
	candidate.SourceQuote = hiddenDetailString(item, "source_quote")
	candidate.Context = hiddenDetailString(item, "context")
	candidate.EdgeEvidence = hiddenDetailInferenceEvidence(item, "edge_evidence")
}

func hiddenDetailStringSlice(item map[string]any, key string) []string {
	if item == nil {
		return nil
	}
	value, ok := item[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return trimmedStringSlice(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, raw := range typed {
			if value, ok := raw.(string); ok {
				out = append(out, value)
			}
		}
		return trimmedStringSlice(out)
	default:
		return nil
	}
}

func hiddenDetailInferenceEvidence(item map[string]any, key string) []authorInferenceEvidence {
	if item == nil {
		return nil
	}
	value, ok := item[key]
	if !ok || value == nil {
		return nil
	}
	var rawItems []map[string]any
	switch typed := value.(type) {
	case []map[string]any:
		rawItems = typed
	case []any:
		for _, raw := range typed {
			if rawMap, ok := raw.(map[string]any); ok {
				rawItems = append(rawItems, rawMap)
			}
		}
	}
	out := make([]authorInferenceEvidence, 0, len(rawItems))
	for _, raw := range rawItems {
		evidence := authorInferenceEvidence{
			From:        hiddenDetailString(raw, "from"),
			To:          hiddenDetailString(raw, "to"),
			FromText:    hiddenDetailString(raw, "from_text"),
			ToText:      hiddenDetailString(raw, "to_text"),
			SourceQuote: hiddenDetailString(raw, "source_quote"),
			Reason:      hiddenDetailString(raw, "reason"),
		}
		if evidence != (authorInferenceEvidence{}) {
			out = append(out, evidence)
		}
	}
	return out
}

func trimmedStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
