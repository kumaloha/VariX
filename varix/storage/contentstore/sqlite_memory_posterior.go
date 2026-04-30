package contentstore

import (
	"context"
	"database/sql"

	"github.com/kumaloha/VariX/varix/memory"
)

type sourceScopeKey struct {
	sourcePlatform   string
	sourceExternalID string
}

type acceptedScopeKey struct {
	userID           string
	sourcePlatform   string
	sourceExternalID string
}

type posteriorNodeState struct {
	node    memory.AcceptedNode
	current memory.PosteriorStateRecord
}

type rowScanner interface {
	Scan(dest ...any) error
}

type rowsQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}
