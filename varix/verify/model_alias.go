package verify

import (
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/model"
)

const (
	Qwen36PlusModel = varixllm.Qwen36PlusModel

	NodeFact              = model.NodeFact
	NodeMechanism         = model.NodeMechanism
	NodeExplicitCondition = model.NodeExplicitCondition
	NodeImplicitCondition = model.NodeImplicitCondition
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
	EdgeExplains = model.EdgeExplains
	EdgePresets  = model.EdgePresets

	FactStatusClearlyTrue  = model.FactStatusClearlyTrue
	FactStatusClearlyFalse = model.FactStatusClearlyFalse
	FactStatusUnverifiable = model.FactStatusUnverifiable

	ExplicitConditionStatusHigh    = model.ExplicitConditionStatusHigh
	ExplicitConditionStatusMedium  = model.ExplicitConditionStatusMedium
	ExplicitConditionStatusLow     = model.ExplicitConditionStatusLow
	ExplicitConditionStatusUnknown = model.ExplicitConditionStatusUnknown

	PredictionStatusResolvedTrue      = model.PredictionStatusResolvedTrue
	PredictionStatusResolvedFalse     = model.PredictionStatusResolvedFalse
	PredictionStatusUnresolved        = model.PredictionStatusUnresolved
	PredictionStatusStaleUnresolved   = model.PredictionStatusStaleUnresolved
	NodeVerificationProved            = model.NodeVerificationProved
	NodeVerificationFalsified         = model.NodeVerificationFalsified
	NodeVerificationWaiting           = model.NodeVerificationWaiting
	PathVerificationSound             = model.PathVerificationSound
	PathVerificationProblem           = model.PathVerificationProblem
	VerificationPassFact              = model.VerificationPassFact
	VerificationPassExplicitCondition = model.VerificationPassExplicitCondition
	VerificationPassImplicitCondition = model.VerificationPassImplicitCondition
	VerificationPassPrediction        = model.VerificationPassPrediction
)

type (
	Bundle                       = model.Bundle
	Output                       = model.Output
	GraphNode                    = model.GraphNode
	TransmissionPath             = model.TransmissionPath
	FactStatus                   = model.FactStatus
	ExplicitConditionStatus      = model.ExplicitConditionStatus
	PredictionStatus             = model.PredictionStatus
	FactCheck                    = model.FactCheck
	ExplicitConditionCheck       = model.ExplicitConditionCheck
	ImplicitConditionCheck       = model.ImplicitConditionCheck
	PredictionCheck              = model.PredictionCheck
	RealizedCheck                = model.RealizedCheck
	FutureConditionCheck         = model.FutureConditionCheck
	NodeVerificationStatus       = model.NodeVerificationStatus
	NodeVerification             = model.NodeVerification
	PathVerificationStatus       = model.PathVerificationStatus
	PathVerification             = model.PathVerification
	VerificationPassKind         = model.VerificationPassKind
	VerificationStageSummary     = model.VerificationStageSummary
	VerificationPassCoverage     = model.VerificationPassCoverage
	VerificationRetrievalSummary = model.VerificationRetrievalSummary
	VerificationPass             = model.VerificationPass
	VerificationCoverageSummary  = model.VerificationCoverageSummary
	Verification                 = model.Verification
)

var (
	BuildQwen36ProviderRequest = varixllm.BuildQwen36ProviderRequest
	BuildBundle                = model.BuildBundle
	CloneStrings               = model.CloneStrings
	FirstNonEmpty              = model.FirstNonEmpty
	NowUTC                     = model.NowUTC
)
