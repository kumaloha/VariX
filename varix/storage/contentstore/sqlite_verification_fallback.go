package contentstore

import (
	"context"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
)

func (s *SQLiteStore) BuildVerificationRecordFromContentSubgraph(ctx context.Context, platform, externalID string) (compile.VerificationRecord, error) {
	subgraph, err := s.GetContentSubgraph(ctx, platform, externalID)
	if err != nil {
		return compile.VerificationRecord{}, err
	}
	verification := compile.Verification{}
	for _, node := range subgraph.Nodes {
		switch node.Kind {
		case graphmodel.NodeKindPrediction:
			status := compile.PredictionStatusUnresolved
			switch node.VerificationStatus {
			case graphmodel.VerificationProved:
				status = compile.PredictionStatusResolvedTrue
			case graphmodel.VerificationDisproved:
				status = compile.PredictionStatusResolvedFalse
			case graphmodel.VerificationUnverifiable:
				status = compile.PredictionStatusStaleUnresolved
			}
			verification.PredictionChecks = append(verification.PredictionChecks, compile.PredictionCheck{NodeID: node.ID, Status: status, Reason: node.VerificationReason, AsOf: parseSQLiteTime(node.VerificationAsOf)})
		default:
			var status compile.FactStatus
			switch node.VerificationStatus {
			case graphmodel.VerificationProved:
				status = compile.FactStatusClearlyTrue
			case graphmodel.VerificationDisproved:
				status = compile.FactStatusClearlyFalse
			case graphmodel.VerificationUnverifiable:
				status = compile.FactStatusUnverifiable
			default:
				continue
			}
			verification.FactChecks = append(verification.FactChecks, compile.FactCheck{NodeID: node.ID, Status: status, Reason: node.VerificationReason})
		}
	}
	verifiedAt := parseSQLiteTime(subgraph.UpdatedAt)
	if verifiedAt.IsZero() {
		verifiedAt = normalizeRecordedTime(verifiedAt)
	}
	verification.VerifiedAt = verifiedAt
	return compile.VerificationRecord{UnitID: subgraph.ArticleID, Source: subgraph.SourcePlatform, ExternalID: subgraph.SourceExternalID, RootExternalID: subgraph.RootExternalID, Model: subgraph.CompileVersion, Verification: verification, VerifiedAt: verifiedAt}, nil
}

func (s *SQLiteStore) ApplyVerificationRecordToContentSubgraph(ctx context.Context, record compile.VerificationRecord) error {
	for _, check := range record.Verification.FactChecks {
		verdict := graphmodel.VerifyVerdict{ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: check.NodeID, Reason: check.Reason, AsOf: record.VerifiedAt.UTC().Format(time.RFC3339)}
		switch check.Status {
		case compile.FactStatusClearlyTrue:
			verdict.Verdict = graphmodel.VerificationProved
		case compile.FactStatusClearlyFalse:
			verdict.Verdict = graphmodel.VerificationDisproved
		case compile.FactStatusUnverifiable:
			verdict.Verdict = graphmodel.VerificationUnverifiable
		default:
			continue
		}
		if err := s.ApplyVerifyVerdictToContentSubgraph(ctx, record.Source, record.ExternalID, verdict); err != nil {
			return err
		}
	}
	for _, check := range record.Verification.PredictionChecks {
		verdict := graphmodel.VerifyVerdict{ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: check.NodeID, Reason: check.Reason, AsOf: record.VerifiedAt.UTC().Format(time.RFC3339)}
		switch check.Status {
		case compile.PredictionStatusResolvedTrue:
			verdict.Verdict = graphmodel.VerificationProved
		case compile.PredictionStatusResolvedFalse:
			verdict.Verdict = graphmodel.VerificationDisproved
		case compile.PredictionStatusStaleUnresolved:
			verdict.Verdict = graphmodel.VerificationUnverifiable
		case compile.PredictionStatusUnresolved:
			verdict.Verdict = graphmodel.VerificationPending
		default:
			continue
		}
		if err := s.ApplyVerifyVerdictToContentSubgraph(ctx, record.Source, record.ExternalID, verdict); err != nil {
			return err
		}
	}
	return nil
}
