# hmux

Minimal routing with middleware over Go's standard `http.ServeMux`.

## Installation

```bash
go get github.com/nikita-shtimenko/hmux
```

Requires Go 1.22+.

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    "net/http"

    "github.com/nikita-shtimenko/hmux"
)

func main() {
    mux := hmux.New()

    // Add middleware
    mux.Use(func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            log.Printf("%s %s", r.Method, r.URL.Path)
            next.ServeHTTP(w, r)
        })
    })

    // Register routes using Go 1.22+ patterns
    mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
        id := r.PathValue("id")
        fmt.Fprintf(w, "User: %s", id)
    })

    // Create route groups
    api := mux.Group("/api/v1")
    api.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprint(w, "OK")
    })

    http.ListenAndServe(":8080", mux)
}
```

## Features

- **Minimal API** — `New`, `Handle`, `HandleFunc`, `Use`, `Group`, `With`, and `Chain`
- **Middleware support** — standard onion model with registration-time wrapping
- **Inline middleware** — `With()` for single-route middleware without creating groups
- **Middleware composition** — `Chain()` for creating reusable middleware stacks
- **Route groups** — hierarchical prefixes with inherited middleware
- **Router interface** — enables testing with mocks and generic router handling
- **Go 1.22+ patterns** — full support for method routing and path parameters
- **Zero dependencies** — only the standard library
- **http.Handler compatible** — use with any stdlib-compatible server

## API

### Mux

```go
mux := hmux.New()                           // Create router
mux.Use(middleware...)                         // Add middleware
mux.Handle(pattern, handler)                   // Register http.Handler
mux.HandleFunc(pattern, func)                  // Register http.HandlerFunc
mux.With(middleware...).HandleFunc(...)        // Inline middleware for single route
group := mux.Group("/prefix")                  // Create route group
mux.Handler()                                  // Access underlying *http.ServeMux
mux.ServeHTTP(w, r)                            // Implement http.Handler
```

### Group

```go
group.Use(middleware...)                       // Add group-specific middleware
group.Handle(pattern, handler)                 // Register with prefix
group.HandleFunc(pattern, func)                // Register with prefix
group.With(middleware...).HandleFunc(...)      // Inline middleware
nested := group.Group("/nested")               // Create nested group
```

### Router Interface

Both `Mux` and `Group` implement the `Router` interface:

```go
type Router interface {
    Handle(pattern string, handler http.Handler)
    HandleFunc(pattern string, handler http.HandlerFunc)
    Use(mw ...func(http.Handler) http.Handler)
    Group(prefix string) Router
    With(mw ...func(http.Handler) http.Handler) Router
}
```

### Middleware

Middleware are standard `func(http.Handler) http.Handler` functions — the same signature used throughout the Go ecosystem. They are applied at registration time. Order matters: `Use(A, B, C)` means requests flow `A → B → C → Handler → C → B → A`.

### Chain

Pre-compose middleware into a single function for reuse:

```go
authStack := hmux.Chain(logging, auth, rateLimit)

mux.With(authStack).HandleFunc("GET /admin", adminHandler)
mux.With(authStack).HandleFunc("POST /admin/users", createUserHandler)
```

## Inline Middleware with With()

Use `With()` to apply middleware to a single route without creating a group:

```go
mux := hmux.New()
mux.Use(logging)  // All routes

// Only /admin gets auth middleware
mux.With(auth).HandleFunc("GET /admin", adminHandler)

// /public has only logging
mux.HandleFunc("GET /public", publicHandler)
```

`With()` can be chained:

```go
mux.With(auth).With(rateLimit).HandleFunc("POST /api", handler)
```

## Patterns

Uses Go 1.22+ `http.ServeMux` pattern syntax:

```go
mux.HandleFunc("/users", h)                    // All methods
mux.HandleFunc("GET /users", h)                // GET only
mux.HandleFunc("GET /users/{id}", h)           // Path parameter
mux.HandleFunc("GET /files/{path...}", h)      // Wildcard
```

Access path parameters with `r.PathValue("id")`.

## Groups

Groups inherit middleware and concatenate prefixes:

```go
mux := hmux.New()
mux.Use(logging)                               // All routes

api := mux.Group("/api")
api.Use(auth)                                  // /api/* routes

v1 := api.Group("/v1")
v1.HandleFunc("GET /users", h)                 // GET /api/v1/users (has logging + auth)
```

## Documentation

See [pkg.go.dev](https://pkg.go.dev/github.com/nikita-shtimenko/hmux) for complete API documentation.

## License

MIT
