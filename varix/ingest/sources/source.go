package sources

import (
	"context"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

type ItemSource interface {
	Platform() types.Platform
	Fetch(ctx context.Context, parsed types.ParsedURL) ([]types.RawContent, error)
}

type Discoverer interface {
	Kind() types.Kind
	Platform() types.Platform
	Discover(ctx context.Context, target types.FollowTarget) ([]types.DiscoveryItem, error)
}
