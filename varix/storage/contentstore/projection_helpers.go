package contentstore

import (
	"context"
	"strings"

	"github.com/kumaloha/VariX/varix/model"
)

func (s *SQLiteStore) resolveCanonicalGraphNodeSubject(ctx context.Context, node model.ContentNode, cache map[string]string) (string, error) {
	subject, err := s.resolveCanonicalSubject(ctx, firstTrimmed(node.SubjectCanonical, node.SubjectText), cache)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(subject), nil
}

func normalizedEventBucket(values ...string) string {
	bucket := firstTrimmed(values...)
	if bucket == "" {
		return "timeless"
	}
	return bucket
}
