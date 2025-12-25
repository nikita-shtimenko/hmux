package hmux

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// recordingMiddleware creates a middleware that appends a marker to
// a shared slice on entry and exit. This enables verification of
// middleware execution order.
func recordingMiddleware(name string, record *[]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*record = append(*record, name+":enter")
			next.ServeHTTP(w, r)
			*record = append(*record, name+":exit")
		})
	}
}

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
	if m.mux == nil {
		t.Error("underlying ServeMux is nil")
	}
	if m.middleware != nil {
		t.Error("middleware should be nil initially")
	}
}

func TestMiddleware_ExecutionOrder(t *testing.T) {
	var record []string
	m := New()
	m.Use(recordingMiddleware("A", &record))
	m.Use(recordingMiddleware("B", &record))
	m.Use(recordingMiddleware("C", &record))

	m.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "handler")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"A:enter", "B:enter", "C:enter", "handler", "C:exit", "B:exit", "A:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("expected %v, got %v", expected, record)
	}
}

func TestMiddleware_SingleUseMultipleArgs(t *testing.T) {
	var record []string
	m := New()
	m.Use(
		recordingMiddleware("A", &record),
		recordingMiddleware("B", &record),
		recordingMiddleware("C", &record),
	)

	m.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "handler")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"A:enter", "B:enter", "C:enter", "handler", "C:exit", "B:exit", "A:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("expected %v, got %v", expected, record)
	}
}

func TestMiddleware_MultipleUseCalls(t *testing.T) {
	var record []string
	m := New()
	m.Use(recordingMiddleware("A", &record))
	m.Use(recordingMiddleware("B", &record))

	m.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "handler")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"A:enter", "B:enter", "handler", "B:exit", "A:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("expected %v, got %v", expected, record)
	}
}

func TestMiddleware_OnlyAffectsSubsequentHandlers(t *testing.T) {
	var record []string
	m := New()

	// Register handler BEFORE middleware
	m.HandleFunc("/before", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "before-handler")
	})

	m.Use(recordingMiddleware("MW", &record))

	// Register handler AFTER middleware
	m.HandleFunc("/after", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "after-handler")
	})

	// Test /before - should NOT have middleware
	record = nil
	req := httptest.NewRequest(http.MethodGet, "/before", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if !slices.Equal(record, []string{"before-handler"}) {
		t.Errorf("/before: expected only handler, got %v", record)
	}

	// Test /after - should have middleware
	record = nil
	req = httptest.NewRequest(http.MethodGet, "/after", nil)
	rec = httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"MW:enter", "after-handler", "MW:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("/after: expected %v, got %v", expected, record)
	}
}

func TestGroup_PrefixPrepended(t *testing.T) {
	// Integration test: verify that groups correctly join prefixes
	// and register with the underlying mux
	m := New()
	g := m.Group("/api")

	var called bool
	g.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called - group prefix not applied correctly")
	}
}

func TestGroup_InheritsParentMiddleware(t *testing.T) {
	var record []string
	m := New()
	m.Use(recordingMiddleware("parent", &record))

	g := m.Group("/api")
	g.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "handler")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"parent:enter", "handler", "parent:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("expected %v, got %v", expected, record)
	}
}

func TestGroup_UseDoesNotAffectParent(t *testing.T) {
	var record []string
	m := New()

	g := m.Group("/api")
	g.Use(recordingMiddleware("group-mw", &record))
	g.HandleFunc("/grouped", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "grouped-handler")
	})

	// Register on parent AFTER group.Use()
	m.HandleFunc("/root", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "root-handler")
	})

	// Test /root - should NOT have group middleware
	record = nil
	req := httptest.NewRequest(http.MethodGet, "/root", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if !slices.Equal(record, []string{"root-handler"}) {
		t.Errorf("/root: expected only handler, got %v", record)
	}

	// Test /api/grouped - should have group middleware
	record = nil
	req = httptest.NewRequest(http.MethodGet, "/api/grouped", nil)
	rec = httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"group-mw:enter", "grouped-handler", "group-mw:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("/api/grouped: expected %v, got %v", expected, record)
	}
}

func TestGroup_NestedPrefixes(t *testing.T) {
	// Integration test: verify that nested groups correctly concatenate prefixes
	m := New()
	api := m.Group("/api")
	v1 := api.Group("/v1")

	var called bool
	v1.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called - nested prefixes not concatenated correctly")
	}
}

func TestGroup_NestedMiddleware(t *testing.T) {
	var record []string
	m := New()
	m.Use(recordingMiddleware("root", &record))

	api := m.Group("/api")
	api.Use(recordingMiddleware("api", &record))

	v1 := api.Group("/v1")
	v1.Use(recordingMiddleware("v1", &record))

	v1.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "handler")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{
		"root:enter", "api:enter", "v1:enter",
		"handler",
		"v1:exit", "api:exit", "root:exit",
	}
	if !slices.Equal(record, expected) {
		t.Errorf("expected %v, got %v", expected, record)
	}
}

func TestJoinPattern(t *testing.T) {
	tests := []struct {
		prefix  string
		pattern string
		want    string
	}{
		// Basic patterns
		{"/api/v1", "/users", "/api/v1/users"},
		{"/api/v1", "GET /users", "GET /api/v1/users"},
		{"/api/", "/users", "/api/users"},
		{"/api", "/users/", "/api/users/"},
		{"/api", "POST /items/{id}", "POST /api/items/{id}"},

		// Edge cases: root and empty prefixes
		{"/", "/test", "/test"},
		{"/", "/users", "/users"},
		{"", "/users", "/users"},

		// Patterns without leading slash
		{"/api", "users", "/api/users"},
		{"/api", "GET users", "GET /api/users"},

		// Various HTTP methods
		{"/api/v1", "DELETE /users/{id}", "DELETE /api/v1/users/{id}"},
		{"/api", "PATCH /users/{id}", "PATCH /api/users/{id}"},
		{"/api", "HEAD /status", "HEAD /api/status"},
		{"/api", "OPTIONS /cors", "OPTIONS /api/cors"},
	}

	for _, tt := range tests {
		got := joinPattern(tt.prefix, tt.pattern)
		if got != tt.want {
			t.Errorf("joinPattern(%q, %q) = %q, want %q", tt.prefix, tt.pattern, got, tt.want)
		}
	}
}

func TestSplitMethodPath(t *testing.T) {
	tests := []struct {
		pattern    string
		wantMethod string
		wantPath   string
	}{
		{"/users", "", "/users"},
		{"GET /users", http.MethodGet, "/users"},
		{"POST /items", http.MethodPost, "/items"},
		{"PUT /things/{id}", http.MethodPut, "/things/{id}"},
		{"DELETE /remove", http.MethodDelete, "/remove"},
		{"PATCH /update", http.MethodPatch, "/update"},
		{"HEAD /check", http.MethodHead, "/check"},
		{"OPTIONS /cors", http.MethodOptions, "/cors"},
		{"CONNECT /proxy", http.MethodConnect, "/proxy"},
		{"TRACE /debug", http.MethodTrace, "/debug"},
		{"UNKNOWN /path", "", "UNKNOWN /path"}, // Not a recognized method
		{"/path with space", "", "/path with space"},
	}

	for _, tt := range tests {
		gotMethod, gotPath := splitMethodPath(tt.pattern)
		if gotMethod != tt.wantMethod || gotPath != tt.wantPath {
			t.Errorf("splitMethodPath(%q) = (%q, %q), want (%q, %q)",
				tt.pattern, gotMethod, gotPath, tt.wantMethod, tt.wantPath)
		}
	}
}

func TestGroup_UseAfterHandle(t *testing.T) {
	var record []string
	m := New()
	g := m.Group("/api")

	// Register handler BEFORE group.Use()
	g.HandleFunc("/before", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "before-handler")
	})

	g.Use(recordingMiddleware("mw", &record))

	// Register handler AFTER group.Use()
	g.HandleFunc("/after", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "after-handler")
	})

	// Test /api/before - should NOT have middleware
	record = nil
	req := httptest.NewRequest(http.MethodGet, "/api/before", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if !slices.Equal(record, []string{"before-handler"}) {
		t.Errorf("/api/before: expected only handler, got %v", record)
	}

	// Test /api/after - should have middleware
	record = nil
	req = httptest.NewRequest(http.MethodGet, "/api/after", nil)
	rec = httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"mw:enter", "after-handler", "mw:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("/api/after: expected %v, got %v", expected, record)
	}
}

func TestSiblingGroups_IndependentMiddleware(t *testing.T) {
	var record []string
	m := New()

	g1 := m.Group("/api1")
	g1.Use(recordingMiddleware("g1-mw", &record))
	g1.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "g1-handler")
	})

	g2 := m.Group("/api2")
	g2.Use(recordingMiddleware("g2-mw", &record))
	g2.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "g2-handler")
	})

	// Test /api1/test - should have g1-mw only
	record = nil
	req := httptest.NewRequest(http.MethodGet, "/api1/test", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"g1-mw:enter", "g1-handler", "g1-mw:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("/api1/test: expected %v, got %v", expected, record)
	}

	// Test /api2/test - should have g2-mw only
	record = nil
	req = httptest.NewRequest(http.MethodGet, "/api2/test", nil)
	rec = httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected = []string{"g2-mw:enter", "g2-handler", "g2-mw:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("/api2/test: expected %v, got %v", expected, record)
	}
}

func TestUse_EmptySlice(t *testing.T) {
	m := New()
	m.Use() // No-op, should not panic

	var called bool
	m.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestMux_Group_InvalidPrefix_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid prefix")
		}
	}()
	m := New()
	m.Group("api") // Missing leading slash
}

func TestGroup_Group_InvalidPrefix_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid prefix")
		}
	}()
	m := New()
	g := m.Group("/api")
	g.Group("v1") // Missing leading slash
}

func TestMux_With(t *testing.T) {
	var record []string
	m := New()
	m.Use(recordingMiddleware("global", &record))

	m.With(recordingMiddleware("special", &record)).HandleFunc("GET /special", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "special-handler")
	})

	m.HandleFunc("GET /normal", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "normal-handler")
	})

	// Test /special - should have both global and special middleware
	record = nil
	req := httptest.NewRequest(http.MethodGet, "/special", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"global:enter", "special:enter", "special-handler", "special:exit", "global:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("/special: expected %v, got %v", expected, record)
	}

	// Test /normal - should have only global middleware
	record = nil
	req = httptest.NewRequest(http.MethodGet, "/normal", nil)
	rec = httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected = []string{"global:enter", "normal-handler", "global:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("/normal: expected %v, got %v", expected, record)
	}
}

func TestMux_With_MultipleMiddleware(t *testing.T) {
	var record []string
	m := New()

	m.With(
		recordingMiddleware("A", &record),
		recordingMiddleware("B", &record),
	).HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "handler")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"A:enter", "B:enter", "handler", "B:exit", "A:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("expected %v, got %v", expected, record)
	}
}

func TestMux_With_DoesNotAffectParent(t *testing.T) {
	var record []string
	m := New()

	// Register via With
	m.With(recordingMiddleware("special", &record)).HandleFunc("GET /with", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "with-handler")
	})

	// Register directly on mux after With
	m.HandleFunc("GET /direct", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "direct-handler")
	})

	// Test /direct - should NOT have special middleware
	record = nil
	req := httptest.NewRequest(http.MethodGet, "/direct", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if !slices.Equal(record, []string{"direct-handler"}) {
		t.Errorf("/direct: expected only handler, got %v", record)
	}
}

func TestGroup_With(t *testing.T) {
	var record []string
	m := New()
	m.Use(recordingMiddleware("global", &record))

	api := m.Group("/api")
	api.Use(recordingMiddleware("api", &record))

	api.With(recordingMiddleware("admin", &record)).HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "admin-handler")
	})

	api.HandleFunc("GET /public", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "public-handler")
	})

	// Test /api/admin - should have global + api + admin middleware
	record = nil
	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"global:enter", "api:enter", "admin:enter", "admin-handler", "admin:exit", "api:exit", "global:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("/api/admin: expected %v, got %v", expected, record)
	}

	// Test /api/public - should have only global + api middleware
	record = nil
	req = httptest.NewRequest(http.MethodGet, "/api/public", nil)
	rec = httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected = []string{"global:enter", "api:enter", "public-handler", "api:exit", "global:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("/api/public: expected %v, got %v", expected, record)
	}
}

func TestWith_Chaining(t *testing.T) {
	var record []string
	m := New()

	m.With(recordingMiddleware("A", &record)).With(recordingMiddleware("B", &record)).HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "handler")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"A:enter", "B:enter", "handler", "B:exit", "A:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("expected %v, got %v", expected, record)
	}
}

func TestRouterInterface(t *testing.T) {
	// Verify both types implement Router at compile time
	var _ Router = New()
	var _ Router = New().Group("/api")
	var _ Router = New().With()
}

func TestMux_Use_NilMiddleware_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil middleware")
		}
	}()
	m := New()
	m.Use(nil)
}

func TestMux_Use_NilMiddlewareInSlice_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil middleware in slice")
		}
	}()
	m := New()
	m.Use(
		func(next http.Handler) http.Handler { return next },
		nil,
		func(next http.Handler) http.Handler { return next },
	)
}

func TestGroup_Use_NilMiddleware_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil middleware")
		}
	}()
	m := New()
	g := m.Group("/api")
	g.Use(nil)
}

func TestHandler(t *testing.T) {
	m := New()
	m.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	h := m.Handler()
	if h == nil {
		t.Fatal("Handler() returned nil")
	}

	// Verify the returned ServeMux works
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %q", rec.Body.String())
	}
}

func TestChain(t *testing.T) {
	var record []string
	m := New()

	chain := Chain(
		recordingMiddleware("A", &record),
		recordingMiddleware("B", &record),
		recordingMiddleware("C", &record),
	)

	m.With(chain).HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "handler")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"A:enter", "B:enter", "C:enter", "handler", "C:exit", "B:exit", "A:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("expected %v, got %v", expected, record)
	}
}

func TestChain_Empty(t *testing.T) {
	m := New()

	chain := Chain()

	var called bool
	m.With(chain).HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestChain_SingleMiddleware(t *testing.T) {
	var record []string
	m := New()

	chain := Chain(recordingMiddleware("A", &record))

	m.With(chain).HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		record = append(record, "handler")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	expected := []string{"A:enter", "handler", "A:exit"}
	if !slices.Equal(record, expected) {
		t.Errorf("expected %v, got %v", expected, record)
	}
}

// Benchmarks
// These benchmarks measure hmux-specific overhead during route registration.
// Request serving (ServeHTTP) benchmarks are omitted because hmux adds zero
// overhead at request time - middleware is wrapped at registration time.

func BenchmarkMux_HandleFunc_Registration(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := New()
		m.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}
}

func BenchmarkMux_HandleFunc_RegistrationWithMiddleware(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := New()
		m.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})
		})
		m.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}
}

func BenchmarkGroup_Creation(b *testing.B) {
	m := New()
	m.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Group("/api")
	}
}

func BenchmarkWith_Creation(b *testing.B) {
	m := New()
	m.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.With(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})
		})
	}
}

func BenchmarkChain_Composition(b *testing.B) {
	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
	mw3 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Chain(mw1, mw2, mw3)
	}
}
