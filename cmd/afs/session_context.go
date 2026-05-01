package main

import "context"

// sessionIDContextKey is the context key under which the active AFS session
// ID is stashed by sync/mount daemons before making control-plane calls.
// The HTTP client reads it and mountes the X-AFS-Session-Id header so the
// control plane can attribute changelog entries.
type sessionIDContextKey struct{}

// withSessionID returns ctx enriched with the given session ID. Empty values
// leave the context untouched.
func withSessionID(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionIDContextKey{}, sessionID)
}

// sessionIDFromContext extracts a session ID previously stashed via
// withSessionID. Returns "" if none present.
func sessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	sid, _ := ctx.Value(sessionIDContextKey{}).(string)
	return sid
}
