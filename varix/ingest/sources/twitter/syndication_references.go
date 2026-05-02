package twitter

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/assemble"
	"github.com/kumaloha/VariX/varix/ingest/provenance"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

func newReferenceResolver(client *http.Client) func(context.Context, string) (string, error) {
	resolver := provenance.NewHTTPResolver(client)
	return resolver.Resolve
}

func resolveReferences(ctx context.Context, resolver func(context.Context, string) (string, error), raw *types.RawContent) {
	if raw == nil || len(raw.References) == 0 || resolver == nil {
		return
	}
	filtered := raw.References[:0]
	for i := range raw.References {
		ref := raw.References[i]
		resolved, err := resolver(ctx, ref.URL)
		if err != nil || strings.TrimSpace(resolved) == "" {
			filtered = append(filtered, ref)
			continue
		}
		ref.URL = strings.TrimSpace(resolved)
		if platform, externalID, label, ok := detectPostReference(ref.URL); ok {
			if platform == raw.Source && externalID == raw.ExternalID {
				continue
			}
			ref.Kind = "post_link"
			ref.Platform = platform
			ref.ExternalID = externalID
			ref.Label = label
		}
		filtered = append(filtered, ref)
	}
	raw.References = filtered
	raw.Content = refreshReferencePlaceholders(raw.Content, raw.References)
}

func refreshReferencePlaceholders(content string, references []types.Reference) string {
	out := content
	for i, ref := range references {
		pattern := regexp.MustCompile(`\[参考#` + fmt.Sprintf("%d", i+1) + ` [^\]]+\]`)
		out = pattern.ReplaceAllString(out, assemble.FormatReferencePlaceholder(i+1, ref))
	}
	out = regexp.MustCompile(`\[参考#\d+ [^\]]+\]`).ReplaceAllStringFunc(out, func(match string) string {
		for i := range references {
			expected := `[参考#` + fmt.Sprintf("%d", i+1)
			if strings.HasPrefix(match, expected) {
				return match
			}
		}
		return ""
	})
	out = regexp.MustCompile(`\n{3,}`).ReplaceAllString(out, "\n\n")
	out = strings.TrimSpace(out)
	return out
}
