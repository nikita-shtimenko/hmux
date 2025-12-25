package hmux_test

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/nikita-shtimenko/hmux"
)

func Example() {
	mux := hmux.New()

	// Add global middleware
	mux.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("%s %s", r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
		})
	})

	// Register routes
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Welcome!")
	})

	// Create API group
	api := mux.Group("/api/v1")
	api.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "OK")
	})

	// Use as http.Handler with proper timeouts
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	_ = srv.ListenAndServe()
}

func Example_middleware() {
	mux := hmux.New()

	// Auth middleware
	auth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	mux.Use(auth)

	mux.HandleFunc("GET /protected", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Secret data")
	})

	// Test without auth
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	fmt.Println(rec.Code)

	// Test with auth
	req = httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	fmt.Println(rec.Body.String())

	// Output:
	// 401
	// Secret data
}

func Example_groups() {
	mux := hmux.New()

	// Public routes
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Public")
	})

	// API v1 group
	v1 := mux.Group("/api/v1")
	v1.HandleFunc("GET /users", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "v1 users")
	})

	// API v2 group
	v2 := mux.Group("/api/v2")
	v2.HandleFunc("GET /users", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "v2 users")
	})

	// Test v1
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	fmt.Println(rec.Body.String())

	// Test v2
	req = httptest.NewRequest(http.MethodGet, "/api/v2/users", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	fmt.Println(rec.Body.String())

	// Output:
	// v1 users
	// v2 users
}

func Example_nestedGroups() {
	mux := hmux.New()

	// Request ID middleware
	requestID := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Request-ID", "123")
			next.ServeHTTP(w, r)
		})
	}

	mux.Use(requestID)

	api := mux.Group("/api")

	// Admin group with additional middleware
	admin := api.Group("/admin")
	admin.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Admin", "true")
			next.ServeHTTP(w, r)
		})
	})

	admin.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Admin Dashboard")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	fmt.Println(rec.Header().Get("X-Request-ID"))
	fmt.Println(rec.Header().Get("X-Admin"))
	fmt.Println(rec.Body.String())

	// Output:
	// 123
	// true
	// Admin Dashboard
}

func Example_inlineMiddleware() {
	mux := hmux.New()

	// Auth middleware
	requireAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	// Public route - no auth
	mux.HandleFunc("GET /public", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Public")
	})

	// Protected route - uses With() for inline middleware
	mux.With(requireAuth).HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Admin")
	})

	// Test public (no auth needed)
	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	fmt.Println(rec.Body.String())

	// Test admin without auth
	req = httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	fmt.Println(rec.Code)

	// Test admin with auth
	req = httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	fmt.Println(rec.Body.String())

	// Output:
	// Public
	// 401
	// Admin
}
