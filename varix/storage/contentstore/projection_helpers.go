package contentstore

import (
	"context"
	"strings"

	"github.com/kumaloha/VariX/varix/graphmodel"
)

func (s *SQLiteStore) resolveCanonicalGraphNodeSubject(ctx context.Context, node graphmodel.GraphNode, cache map[string]string) (string, error) {
	subject, err := s.resolveCanonicalSubject(ctx, strings.TrimSpace(firstNonEmpty(node.SubjectCanonical, node.SubjectText)), cache)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(subject), nil
}

func normalizedEventBucket(values ...string) string {
	bucket := strings.TrimSpace(firstNonEmpty(values...))
	if bucket == "" {
		return "timeless"
	}
	return bucket
}
