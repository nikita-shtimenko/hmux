package hmux

import (
	"net/http"
	"strings"
)

// Group represents a collection of routes that share a common prefix
// and middleware stack. Groups are created via Mux.Group() or Group.Group()
// and allow hierarchical route organization without affecting the parent
// Mux or sibling groups.
type Group struct {
	mux        *Mux
	prefix     string
	middleware []func(http.Handler) http.Handler
}

// Verify Group implements Router interface.
var _ Router = (*Group)(nil)

// Handle registers the handler for the given pattern on this group.
// The final pattern is formed by joining the group's prefix with the
// provided pattern. The handler is wrapped with all middleware in
// this group's stack at the time of this call.
//
// The pattern follows Go 1.22+ syntax. For example, with a group prefix
// of "/api" and pattern "GET /users", the handler is registered at
// "GET /api/users".
func (g *Group) Handle(pattern string, handler http.Handler) {
	fullPattern := joinPattern(g.prefix, pattern)
	g.mux.mux.Handle(fullPattern, wrap(handler, g.middleware))
}

// HandleFunc registers the handler function for the given pattern on
// this group. The final pattern is formed by joining the group's prefix
// with the provided pattern. The handler is wrapped with all middleware
// in this group's stack at the time of this call.
func (g *Group) HandleFunc(pattern string, handler http.HandlerFunc) {
	g.Handle(pattern, handler)
}

// Use appends middleware to this group. Only handlers registered on this
// group after this call will be wrapped with these middleware. Middleware
// added here does not affect the parent Mux or sibling groups.
//
// If Use(A, B, C) is called, then for a subsequent handler H, requests
// flow: A → B → C → H → C → B → A.
//
// Use panics if any middleware is nil.
func (g *Group) Use(mw ...func(http.Handler) http.Handler) {
	for _, fn := range mw {
		if fn == nil {
			panic("hmux: nil middleware passed to Use")
		}
	}

	g.middleware = append(g.middleware, mw...)
}

// Group creates a nested group with a concatenated prefix. The new group
// inherits a copy of this group's current middleware. The nested group's
// prefix is formed by joining this group's prefix with the provided prefix.
//
// For example, if a group has prefix "/api" and Group("/v1") is called,
// the nested group has prefix "/api/v1".
//
// Group panics if prefix is non-empty and does not start with "/".
func (g *Group) Group(prefix string) Router {
	if prefix != "" && !strings.HasPrefix(prefix, "/") {
		panic("hmux: group prefix must be empty or start with /")
	}

	mw := make([]func(http.Handler) http.Handler, len(g.middleware))
	copy(mw, g.middleware)

	return &Group{
		mux:        g.mux,
		prefix:     joinPattern(g.prefix, prefix),
		middleware: mw,
	}
}

// With returns a new Router with the given middleware appended to
// this group's middleware stack. The returned Router has the same
// prefix as this group. This is useful for applying middleware to
// a single route without creating a nested group.
//
// Example:
//
//	api := mux.Group("/api")
//	api.With(authMiddleware).HandleFunc("GET /admin", adminHandler)
func (g *Group) With(mw ...func(http.Handler) http.Handler) Router {
	newG := g.Group("")
	newG.Use(mw...)

	return newG
}
