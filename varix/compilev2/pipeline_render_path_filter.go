package compilev2

import "strings"

func filterCyclicRenderPaths(paths []renderedPath) []renderedPath {
	if len(paths) < 2 {
		return paths
	}
	reaches := map[string]map[string]struct{}{}
	out := make([]renderedPath, 0, len(paths))
	for _, path := range paths {
		nodeIDs := renderedPathNodeIDs(path)
		if len(nodeIDs) < 2 {
			out = append(out, path)
			continue
		}
		if renderedPathHasCycle(nodeIDs, reaches) {
			continue
		}
		out = append(out, path)
		for i := 0; i+1 < len(nodeIDs); i++ {
			addReachability(reaches, nodeIDs[i], nodeIDs[i+1])
		}
	}
	if len(out) == 0 {
		return paths
	}
	return out
}

func renderedPathNodeIDs(path renderedPath) []string {
	nodeIDs := make([]string, 0, len(path.steps)+2)
	if id := strings.TrimSpace(path.driver.ID); id != "" {
		nodeIDs = append(nodeIDs, id)
	}
	for _, step := range path.steps {
		if id := strings.TrimSpace(step.ID); id != "" {
			nodeIDs = append(nodeIDs, id)
		}
	}
	if id := strings.TrimSpace(path.target.ID); id != "" {
		nodeIDs = append(nodeIDs, id)
	}
	return nodeIDs
}

func renderedPathHasCycle(nodeIDs []string, reaches map[string]map[string]struct{}) bool {
	seen := map[string]struct{}{}
	for _, id := range nodeIDs {
		if _, ok := seen[id]; ok {
			return true
		}
		seen[id] = struct{}{}
	}
	for i := 0; i+1 < len(nodeIDs); i++ {
		for j := i + 1; j < len(nodeIDs); j++ {
			if pathReachable(reaches, nodeIDs[j], nodeIDs[i]) {
				return true
			}
		}
	}
	return false
}

func pathReachable(reaches map[string]map[string]struct{}, from, to string) bool {
	if from == to {
		return true
	}
	_, ok := reaches[from][to]
	return ok
}

func addReachability(reaches map[string]map[string]struct{}, from, to string) {
	ensureReachSet := func(id string) map[string]struct{} {
		if reaches[id] == nil {
			reaches[id] = map[string]struct{}{}
		}
		return reaches[id]
	}
	fromSet := ensureReachSet(from)
	fromSet[to] = struct{}{}
	for next := range reaches[to] {
		fromSet[next] = struct{}{}
	}
	for source, targets := range reaches {
		if source == from {
			continue
		}
		if _, ok := targets[from]; !ok {
			continue
		}
		targets[to] = struct{}{}
		for next := range reaches[to] {
			targets[next] = struct{}{}
		}
	}
}
