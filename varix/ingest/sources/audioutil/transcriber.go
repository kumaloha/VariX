package audioutil

import (
	"errors"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"time"
)

var (
	ErrPayloadTooLarge = errors.New("payload too large")
	ErrRateLimited     = errors.New("rate limited")
)

const (
	defaultMaxUploadBytes = 20 * 1024 * 1024
	defaultSegmentSeconds = 10 * 60

	MaxRemoteVideoBytes    = 50 << 20         // 50 MB
	MaxRemoteVideoDuration = 10 * time.Minute // 10 minutes
)

type RemoteVideoOptions struct {
	MaxDownloadBytes int64
	MaxDuration      time.Duration
}

type RemoteVideoResult struct {
	Transcript            string
	TranscriptMethod      string
	TranscriptDiagnostics []types.TranscriptDiagnostic
}

type splitArtifacts struct {
	Parts      []string
	CleanupDir string
}

// ExecProbe and ExecExtract are overridable for testing from external packages.

var (
	ExecProbe   = probeDurationExec
	ExecExtract = extractAudioExec
)
