// Package hmux provides a minimal routing layer over Go's standard
// http.ServeMux with support for middleware and route groups. It leverages
// Go 1.22+ enhanced routing patterns while adding zero-overhead middleware
// composition at registration time.
//
// # Concurrency Safety
//
// Like http.ServeMux, route registration methods (Handle, HandleFunc, Use)
// are not safe for concurrent use. Register all routes during program
// initialization before starting the server.
//
// Correct usage:
//
//	func main() {
//	    mux := hmux.New()
//	    mux.Use(loggingMiddleware)          // Register routes first
//	    mux.HandleFunc("GET /users", handler)
//	    http.ListenAndServe(":8080", mux)    // Then start server
//	}
//
// Incorrect usage (data race):
//
//	func main() {
//	    mux := hmux.New()
//	    go http.ListenAndServe(":8080", mux) // Server started
//	    mux.HandleFunc("GET /users", handler) // Concurrent registration - UNSAFE!
//	}
//
// Once all routes are registered, ServeHTTP is safe for concurrent use.
//
// # Limitations
//
// Host-based patterns are not supported in route groups. When using groups,
// patterns should be path-only. For example:
//
//	// Correct - path-only pattern
//	api := mux.Group("/api")
//	api.HandleFunc("/users", handler)           // ✓ Works: GET /api/users
//	api.HandleFunc("GET /users/{id}", handler)  // ✓ Works: GET /api/users/{id}
//
//	// Incorrect - host-based pattern
//	api.HandleFunc("example.com/users", handler)  // ✗ Creates malformed pattern
//
// If you need host-based routing, register patterns directly on the Mux
// without using groups:
//
//	mux.HandleFunc("example.com/users", handler)  // ✓ Works
package hmux

import (
	"net/http"
	"strings"
)

// Mux is an HTTP request multiplexer with middleware support. It wraps
// the standard library's http.ServeMux, adding middleware composition
// and route grouping capabilities while maintaining full compatibility
// with Go 1.22+ routing patterns.
type Mux struct {
	mux        *http.ServeMux
	middleware []func(http.Handler) http.Handler
}

// Verify Mux implements Router interface.
var _ Router = (*Mux)(nil)

// New creates and returns a new Mux instance backed by an http.ServeMux.
// The returned Mux has no middleware configured and is ready to register
// handlers.
func New() *Mux {
	return &Mux{
		mux:        http.NewServeMux(),
		middleware: nil,
	}
}

// Handle registers the handler for the given pattern. The handler is
// wrapped with all middleware registered via Use() at the time of this
// call. The pattern follows Go 1.22+ syntax including method prefixes
// (e.g., "GET /users/{id}").
//
// Handle panics if the pattern is invalid, already registered, or if
// handler is nil. This matches http.ServeMux behavior.
func (m *Mux) Handle(pattern string, handler http.Handler) {
	m.mux.Handle(pattern, m.wrap(handler))
}

// HandleFunc registers the handler function for the given pattern.
// The handler is wrapped with all middleware registered via Use() at
// the time of this call. The pattern follows Go 1.22+ syntax including
// method prefixes (e.g., "GET /users/{id}").
//
// HandleFunc panics if the pattern is invalid or already registered.
// This matches http.ServeMux behavior.
func (m *Mux) HandleFunc(pattern string, handler http.HandlerFunc) {
	m.Handle(pattern, handler)
}

// Use appends middleware to the Mux. Only handlers registered after
// this call will be wrapped with these middleware. Multiple calls to
// Use accumulate middleware. If Use(A, B, C) is called, then for a
// subsequent handler H, requests flow: A → B → C → H → C → B → A.
//
// Use panics if any middleware is nil.
func (m *Mux) Use(mw ...func(http.Handler) http.Handler) {
	for _, fn := range mw {
		if fn == nil {
			panic("hmux: nil middleware passed to Use")
		}
	}

	m.middleware = append(m.middleware, mw...)
}

// Group creates a new route group with the given prefix. The group
// inherits a copy of the Mux's current middleware. Handlers registered
// on the group will have their patterns prefixed and will include any
// middleware added to the group via Group.Use().
//
// The prefix must be empty or start with "/". It will be joined with
// handler patterns to form the final route. For example, a group with
// prefix "/api" and a handler pattern "GET /users" becomes "GET /api/users".
//
// Group panics if prefix is non-empty and does not start with "/".
func (m *Mux) Group(prefix string) Router {
	if prefix != "" && !strings.HasPrefix(prefix, "/") {
		panic("hmux: group prefix must be empty or start with /")
	}

	mw := make([]func(http.Handler) http.Handler, len(m.middleware))
	copy(mw, m.middleware)

	return &Group{
		mux:        m,
		prefix:     prefix,
		middleware: mw,
	}
}

// With returns a new Router with the given middleware appended to
// the Mux's current middleware stack. The returned Router has no
// prefix, so patterns are registered as-is. This is useful for
// applying middleware to a single route without creating a group.
//
// Example:
//
//	mux.With(authMiddleware).HandleFunc("GET /admin", adminHandler)
func (m *Mux) With(mw ...func(http.Handler) http.Handler) Router {
	g := m.Group("")
	g.Use(mw...)

	return g
}

// ServeHTTP dispatches the request to the handler whose pattern most
// closely matches the request URL. This method delegates directly to
// the underlying http.ServeMux.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mux.ServeHTTP(w, r)
}

// Handler returns the underlying http.ServeMux. This can be useful for
// debugging, introspection, or integration with tools that require
// direct access to the ServeMux.
//
// WARNING: Handlers registered directly on the returned ServeMux will
// bypass all middleware registered with Use(). Only use this method for
// debugging or when you specifically need to bypass middleware. For normal
// route registration, use Handle() or HandleFunc() instead.
func (m *Mux) Handler() *http.ServeMux {
	return m.mux
}

// Chain composes multiple middleware into a single middleware function.
// The returned middleware applies the given middleware in order, producing
// the standard onion model. For Chain(A, B, C), requests flow:
// A → B → C → handler → C → B → A.
//
// This is useful for creating reusable middleware stacks:
//
//	authStack := hmux.Chain(logging, auth, rateLimit)
//	mux.With(authStack).HandleFunc("GET /admin", adminHandler)
func Chain(mw ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return wrap(h, mw)
	}
}

// wrap applies all registered middleware to the handler in reverse order,
// producing the standard onion model execution pattern.
func (m *Mux) wrap(h http.Handler) http.Handler {
	return wrap(h, m.middleware)
}

// wrap applies middleware to a handler in reverse order, producing the
// standard "onion" model where the first middleware in the slice is the
// outermost layer. For middleware [A, B, C] and handler H, requests flow:
// A → B → C → H → C → B → A.
//
// This is achieved by wrapping in reverse: C wraps H, B wraps that result,
// and A wraps the final result.
func wrap(h http.Handler, mw []func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}

	return h
}

// joinPattern combines a group prefix with a handler pattern, correctly
// handling method prefixes in Go 1.22+ routing syntax.
//
// Examples:
//   - prefix="/api", pattern="/users" → "/api/users"
//   - prefix="/api", pattern="GET /users" → "GET /api/users"
//   - prefix="/api/", pattern="/users" → "/api/users"
func joinPattern(prefix, pattern string) string {
	method, path := splitMethodPath(pattern)

	// Normalize: remove trailing slash from prefix, ensure path starts with /
	prefix = strings.TrimSuffix(prefix, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	joined := prefix + path

	if method != "" {
		return method + " " + joined
	}

	return joined
}

// splitMethodPath separates an optional HTTP method prefix from the path
// portion of a pattern.
//
// Examples:
//   - "GET /users" → ("GET", "/users")
//   - "/users" → ("", "/users")
//   - "POST /items/{id}" → ("POST", "/items/{id}")
func splitMethodPath(pattern string) (method, path string) {
	method, path, found := strings.Cut(pattern, " ")
	if !found {
		return "", pattern
	}

	switch method {
	case http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
		http.MethodHead,
		http.MethodOptions,
		http.MethodConnect,
		http.MethodTrace:

		return method, path
	default:
		return "", pattern
	}
}
