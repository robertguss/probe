package engine

import "context"

// Version is the probe release string embedded in every envelope.
const Version = "0.1.0"

type versionData struct {
	Version string `json:"version"`
}

func (e *Engine) version(_ context.Context) Result {
	return e.ok("version", versionData{Version: Version})
}
