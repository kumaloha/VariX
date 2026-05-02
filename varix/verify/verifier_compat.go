package verify

func toRealizedChecks(checks []FactCheck) []RealizedCheck {
	out := make([]RealizedCheck, 0, len(checks))
	for _, check := range checks {
		out = append(out, RealizedCheck{
			NodeID: check.NodeID,
			Status: check.Status,
			Reason: check.Reason,
		})
	}
	return out
}

func toFutureConditionChecksFromPredictions(checks []PredictionCheck) []FutureConditionCheck {
	out := make([]FutureConditionCheck, 0, len(checks))
	for _, check := range checks {
		out = append(out, FutureConditionCheck{
			NodeID: check.NodeID,
			Status: "prediction:" + string(check.Status),
			Reason: check.Reason,
			AsOf:   check.AsOf,
		})
	}
	return out
}

func toFutureConditionChecksFromExplicit(checks []ExplicitConditionCheck) []FutureConditionCheck {
	out := make([]FutureConditionCheck, 0, len(checks))
	for _, check := range checks {
		out = append(out, FutureConditionCheck{
			NodeID: check.NodeID,
			Status: "explicit_condition:" + string(check.Status),
			Reason: check.Reason,
		})
	}
	return out
}

func toFutureConditionChecksFromImplicit(checks []ImplicitConditionCheck) []FutureConditionCheck {
	out := make([]FutureConditionCheck, 0, len(checks))
	for _, check := range checks {
		out = append(out, FutureConditionCheck{
			NodeID: check.NodeID,
			Status: "implicit_condition:" + string(check.Status),
			Reason: check.Reason,
		})
	}
	return out
}

func filterCompatibilityFactChecks(nodes []GraphNode, checks []FactCheck) []FactCheck {
	compatibilityEligible := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if isObservationLikeVerifierNode(node) {
			compatibilityEligible[node.ID] = struct{}{}
		}
	}
	out := make([]FactCheck, 0, len(checks))
	for _, check := range checks {
		if _, ok := compatibilityEligible[check.NodeID]; ok {
			out = append(out, check)
		}
	}
	return out
}

func isObservationLikeVerifierNode(node GraphNode) bool {
	return node.Form == NodeFormObservation || node.Kind == NodeFact || node.Kind == NodeMechanism
}
