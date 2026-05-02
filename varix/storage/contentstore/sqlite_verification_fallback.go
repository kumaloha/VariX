package contentstore

import (
	"context"
	"time"

	"github.com/kumaloha/VariX/varix/model"
)

func (s *SQLiteStore) BuildVerificationRecordFromContentSubgraph(ctx context.Context, platform, externalID string) (model.VerificationRecord, error) {
	subgraph, err := s.GetContentSubgraph(ctx, platform, externalID)
	if err != nil {
		return model.VerificationRecord{}, err
	}
	verification := model.Verification{}
	for _, node := range subgraph.Nodes {
		switch node.Kind {
		case model.NodeKindPrediction:
			status := model.PredictionStatusUnresolved
			switch node.VerificationStatus {
			case model.VerificationProved:
				status = model.PredictionStatusResolvedTrue
			case model.VerificationDisproved:
				status = model.PredictionStatusResolvedFalse
			case model.VerificationUnverifiable:
				status = model.PredictionStatusStaleUnresolved
			}
			verification.PredictionChecks = append(verification.PredictionChecks, model.PredictionCheck{NodeID: node.ID, Status: status, Reason: node.VerificationReason, AsOf: parseSQLiteTime(node.VerificationAsOf)})
		default:
			var status model.FactStatus
			switch node.VerificationStatus {
			case model.VerificationProved:
				status = model.FactStatusClearlyTrue
			case model.VerificationDisproved:
				status = model.FactStatusClearlyFalse
			case model.VerificationUnverifiable:
				status = model.FactStatusUnverifiable
			default:
				continue
			}
			verification.FactChecks = append(verification.FactChecks, model.FactCheck{NodeID: node.ID, Status: status, Reason: node.VerificationReason})
		}
	}
	verifiedAt := parseSQLiteTime(subgraph.UpdatedAt)
	if verifiedAt.IsZero() {
		verifiedAt = normalizeRecordedTime(verifiedAt)
	}
	verification.VerifiedAt = verifiedAt
	return model.VerificationRecord{UnitID: subgraph.ArticleID, Source: subgraph.SourcePlatform, ExternalID: subgraph.SourceExternalID, RootExternalID: subgraph.RootExternalID, Model: subgraph.CompileVersion, Verification: verification, VerifiedAt: verifiedAt}, nil
}

func (s *SQLiteStore) ApplyVerificationRecordToContentSubgraph(ctx context.Context, record model.VerificationRecord) error {
	for _, check := range record.Verification.FactChecks {
		verdict := model.VerifyVerdict{ObjectType: model.VerifyQueueObjectNode, ObjectID: check.NodeID, Reason: check.Reason, AsOf: record.VerifiedAt.UTC().Format(time.RFC3339)}
		switch check.Status {
		case model.FactStatusClearlyTrue:
			verdict.Verdict = model.VerificationProved
		case model.FactStatusClearlyFalse:
			verdict.Verdict = model.VerificationDisproved
		case model.FactStatusUnverifiable:
			verdict.Verdict = model.VerificationUnverifiable
		default:
			continue
		}
		if err := s.ApplyVerifyVerdictToContentSubgraph(ctx, record.Source, record.ExternalID, verdict); err != nil {
			return err
		}
	}
	for _, check := range record.Verification.PredictionChecks {
		verdict := model.VerifyVerdict{ObjectType: model.VerifyQueueObjectNode, ObjectID: check.NodeID, Reason: check.Reason, AsOf: record.VerifiedAt.UTC().Format(time.RFC3339)}
		switch check.Status {
		case model.PredictionStatusResolvedTrue:
			verdict.Verdict = model.VerificationProved
		case model.PredictionStatusResolvedFalse:
			verdict.Verdict = model.VerificationDisproved
		case model.PredictionStatusStaleUnresolved:
			verdict.Verdict = model.VerificationUnverifiable
		case model.PredictionStatusUnresolved:
			verdict.Verdict = model.VerificationPending
		default:
			continue
		}
		if err := s.ApplyVerifyVerdictToContentSubgraph(ctx, record.Source, record.ExternalID, verdict); err != nil {
			return err
		}
	}
	return nil
}
