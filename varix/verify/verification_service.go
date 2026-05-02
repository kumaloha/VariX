package verify

import (
	"context"
	"strings"
)

type VerificationService interface {
	Verify(ctx context.Context, bundle Bundle, output Output) (Verification, error)
}

type verificationService struct {
	runtime verifierCall
	model   string
	prompts *promptRegistry
}

func NewVerificationService(rt verifierCall, model string, prompts *promptRegistry) VerificationService {
	if prompts == nil {
		prompts = newPromptRegistry("")
	}
	return &verificationService{
		runtime: rt,
		model:   strings.TrimSpace(model),
		prompts: prompts,
	}
}

func (s *verificationService) Verify(ctx context.Context, bundle Bundle, output Output) (Verification, error) {
	return runVerifier(ctx, s.runtime, s.model, s.prompts, bundle, output)
}

func projectVerification(output Output, verification Verification) Output {
	output.Verification = verification
	return output
}
