package compile

import "github.com/kumaloha/VariX/varix/model"

// Compatibility aliases for compile-facing callers. Runtime packages should
// import model directly for shared DTOs; boundary tests keep that split honest.
const (
	NodeFact              = model.NodeFact
	NodeExplicitCondition = model.NodeExplicitCondition
	NodeImplicitCondition = model.NodeImplicitCondition
	NodeMechanism         = model.NodeMechanism
	NodeAssumption        = model.NodeAssumption
	NodeConclusion        = model.NodeConclusion
	NodePrediction        = model.NodePrediction

	NodeFormObservation = model.NodeFormObservation
	NodeFormCondition   = model.NodeFormCondition
	NodeFormJudgment    = model.NodeFormJudgment
	NodeFormForecast    = model.NodeFormForecast

	NodeFunctionSupport      = model.NodeFunctionSupport
	NodeFunctionTransmission = model.NodeFunctionTransmission
	NodeFunctionClaim        = model.NodeFunctionClaim

	EdgePositive = model.EdgePositive
	EdgeDerives  = model.EdgeDerives
	EdgePresets  = model.EdgePresets
	EdgeExplains = model.EdgeExplains

	AuthorClaimSupported      = model.AuthorClaimSupported
	AuthorClaimContradicted   = model.AuthorClaimContradicted
	AuthorClaimUnverified     = model.AuthorClaimUnverified
	AuthorClaimInterpretive   = model.AuthorClaimInterpretive
	AuthorClaimNotAuthorClaim = model.AuthorClaimNotAuthorClaim

	AuthorInferenceSound              = model.AuthorInferenceSound
	AuthorInferenceWeak               = model.AuthorInferenceWeak
	AuthorInferenceUnsupportedJump    = model.AuthorInferenceUnsupportedJump
	AuthorInferenceNotAuthorInference = model.AuthorInferenceNotAuthorInference

	FactStatusClearlyTrue  = model.FactStatusClearlyTrue
	FactStatusClearlyFalse = model.FactStatusClearlyFalse
	FactStatusUnverifiable = model.FactStatusUnverifiable

	ExplicitConditionStatusHigh    = model.ExplicitConditionStatusHigh
	ExplicitConditionStatusMedium  = model.ExplicitConditionStatusMedium
	ExplicitConditionStatusLow     = model.ExplicitConditionStatusLow
	ExplicitConditionStatusUnknown = model.ExplicitConditionStatusUnknown

	PredictionStatusUnresolved             = model.PredictionStatusUnresolved
	PredictionStatusResolvedTrue           = model.PredictionStatusResolvedTrue
	PredictionStatusResolvedFalse          = model.PredictionStatusResolvedFalse
	PredictionStatusStaleUnresolved        = model.PredictionStatusStaleUnresolved
	NodeVerificationProved                 = model.NodeVerificationProved
	NodeVerificationFalsified              = model.NodeVerificationFalsified
	NodeVerificationWaiting                = model.NodeVerificationWaiting
	PathVerificationSound                  = model.PathVerificationSound
	PathVerificationProblem                = model.PathVerificationProblem
	DeclarationVerificationProved          = model.DeclarationVerificationProved
	DeclarationVerificationOverclaimed     = model.DeclarationVerificationOverclaimed
	DeclarationVerificationInferredOnly    = model.DeclarationVerificationInferredOnly
	DeclarationVerificationSpeakerMismatch = model.DeclarationVerificationSpeakerMismatch
	DeclarationVerificationConditionLost   = model.DeclarationVerificationConditionLost
	DeclarationVerificationScopeMismatch   = model.DeclarationVerificationScopeMismatch
	VerificationPassFact                   = model.VerificationPassFact
	VerificationPassExplicitCondition      = model.VerificationPassExplicitCondition
	VerificationPassImplicitCondition      = model.VerificationPassImplicitCondition
	VerificationPassPrediction             = model.VerificationPassPrediction
)

type (
	Bundle = model.Bundle

	NodeKind     = model.NodeKind
	NodeForm     = model.NodeForm
	NodeFunction = model.NodeFunction
	EdgeKind     = model.EdgeKind
	GraphNode    = model.GraphNode
	GraphEdge    = model.GraphEdge

	ReasoningGraph   = model.ReasoningGraph
	Declaration      = model.Declaration
	SemanticUnit     = model.SemanticUnit
	Ledger           = model.Ledger
	LedgerItem       = model.LedgerItem
	BriefItem        = model.BriefItem
	TransmissionPath = model.TransmissionPath
	Branch           = model.Branch
	HiddenDetails    = model.HiddenDetails
	Output           = model.Output
	Record           = model.Record
	RecordMetrics    = model.RecordMetrics

	AuthorClaimStatus               = model.AuthorClaimStatus
	AuthorClaimCheck                = model.AuthorClaimCheck
	AuthorEvidenceRequirement       = model.AuthorEvidenceRequirement
	AuthorSubclaim                  = model.AuthorSubclaim
	AuthorInferenceStatus           = model.AuthorInferenceStatus
	AuthorInferenceCheck            = model.AuthorInferenceCheck
	AuthorValidationSummary         = model.AuthorValidationSummary
	AuthorVerificationPlan          = model.AuthorVerificationPlan
	AuthorClaimVerificationPlan     = model.AuthorClaimVerificationPlan
	AuthorAtomicEvidenceSpec        = model.AuthorAtomicEvidenceSpec
	AuthorInferenceVerificationPlan = model.AuthorInferenceVerificationPlan
	AuthorValidation                = model.AuthorValidation

	NodeExtractionOutput      = model.NodeExtractionOutput
	DriverTargetOutput        = model.DriverTargetOutput
	FullGraphOutput           = model.FullGraphOutput
	TransmissionPathOutput    = model.TransmissionPathOutput
	EvidenceExplanationOutput = model.EvidenceExplanationOutput
	UnifiedCompileOutput      = model.UnifiedCompileOutput
	ThesisOutput              = model.ThesisOutput

	FactStatus              = model.FactStatus
	ExplicitConditionStatus = model.ExplicitConditionStatus
	PredictionStatus        = model.PredictionStatus
	FactCheck               = model.FactCheck
	ExplicitConditionCheck  = model.ExplicitConditionCheck
	ImplicitConditionCheck  = model.ImplicitConditionCheck
	PredictionCheck         = model.PredictionCheck
	RealizedCheck           = model.RealizedCheck
	FutureConditionCheck    = model.FutureConditionCheck

	NodeVerificationStatus        = model.NodeVerificationStatus
	NodeVerification              = model.NodeVerification
	PathVerificationStatus        = model.PathVerificationStatus
	PathVerification              = model.PathVerification
	DeclarationVerificationStatus = model.DeclarationVerificationStatus
	DeclarationVerification       = model.DeclarationVerification
	VerificationPassKind          = model.VerificationPassKind
	VerificationStageSummary      = model.VerificationStageSummary
	VerificationPassCoverage      = model.VerificationPassCoverage
	VerificationRetrievalSummary  = model.VerificationRetrievalSummary
	VerificationPass              = model.VerificationPass
	VerificationCoverageSummary   = model.VerificationCoverageSummary
	Verification                  = model.Verification
	VerificationRecord            = model.VerificationRecord
)

var (
	BuildBundle               = model.BuildBundle
	ParseOutput               = model.ParseOutput
	ParseNodeExtractionOutput = model.ParseNodeExtractionOutput
	ParseFullGraphOutput      = model.ParseFullGraphOutput
	ParseUnifiedCompileOutput = model.ParseUnifiedCompileOutput
	ParseThesisOutput         = model.ParseThesisOutput
	SplitParallelDrivers      = model.SplitParallelDrivers
	CloneStrings              = model.CloneStrings
	FirstNonEmpty             = model.FirstNonEmpty
	HasDistinctNonEmptyPair   = model.HasDistinctNonEmptyPair
	NowUTC                    = model.NowUTC
	DurationToMilliseconds    = model.DurationToMilliseconds
)
