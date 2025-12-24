package hmux

import "net/http"

// Router is the interface implemented by both Mux and Group. It provides
// methods for registering handlers, adding middleware, and creating
// sub-groups. This interface enables testing with mock routers and
// writing functions that accept either a Mux or Group.
type Router interface {
	// Handle registers the handler for the given pattern.
	Handle(pattern string, handler http.Handler)

	// HandleFunc registers the handler function for the given pattern.
	HandleFunc(pattern string, handler http.HandlerFunc)

	// Use appends middleware to the router's middleware stack.
	// Only handlers registered after this call will use the middleware.
	Use(mw ...func(http.Handler) http.Handler)

	// Group creates a new route group with the given prefix.
	// The group inherits a copy of the current middleware stack.
	Group(prefix string) Router

	// With returns a new Router with the given middleware appended
	// to the current middleware stack. Useful for applying middleware
	// to a single route without creating a named group.
	With(mw ...func(http.Handler) http.Handler) Router
}
