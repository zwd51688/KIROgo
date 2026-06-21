package pool

import (
	"kiro-go/config"
	"testing"
)

// ---------------------------------------------------------------------------
// RoutingScope.allows
// ---------------------------------------------------------------------------

func TestNilScopeAllowsEverything(t *testing.T) {
	var s *RoutingScope // nil
	if !s.allows("anything") {
		t.Fatal("nil scope must allow all account IDs")
	}
}

func TestBoundScopeAllowsOnlyListedAccounts(t *testing.T) {
	s := NewBoundScope([]string{"a", "b"})
	if s == nil {
		t.Fatal("expected non-nil scope for non-empty bound list")
	}
	if !s.allows("a") || !s.allows("b") {
		t.Fatal("bound scope must allow listed accounts")
	}
	if s.allows("c") {
		t.Fatal("bound scope must reject unlisted account c")
	}
}

func TestNewBoundScopeEmptyReturnsNil(t *testing.T) {
	if s := NewBoundScope(nil); s != nil {
		t.Fatalf("expected nil scope for nil bound list, got %#v", s)
	}
	if s := NewBoundScope([]string{}); s != nil {
		t.Fatalf("expected nil scope for empty bound list, got %#v", s)
	}
	// All-blank IDs are filtered out, leaving an empty allow-set → nil scope.
	if s := NewBoundScope([]string{""}); s != nil {
		t.Fatalf("expected nil scope when all bound IDs are blank, got %#v", s)
	}
}

func TestSharedScopeExcludesExclusiveAccounts(t *testing.T) {
	exclusive := map[string]bool{"x": true, "y": true}
	s := NewSharedScope(exclusive)
	if s == nil {
		t.Fatal("expected non-nil scope when exclusive set is non-empty")
	}
	if s.allows("x") || s.allows("y") {
		t.Fatal("shared scope must reject exclusive accounts")
	}
	if !s.allows("shared") {
		t.Fatal("shared scope must allow non-exclusive account")
	}
}

func TestNewSharedScopeEmptyReturnsNil(t *testing.T) {
	if s := NewSharedScope(nil); s != nil {
		t.Fatalf("expected nil scope when no account is exclusive, got %#v", s)
	}
	if s := NewSharedScope(map[string]bool{}); s != nil {
		t.Fatalf("expected nil scope for empty exclusive set, got %#v", s)
	}
}

// ---------------------------------------------------------------------------
// GetNextExcludingScoped
// ---------------------------------------------------------------------------

func TestGetNextExcludingScopedBoundRestrictsToBoundAccounts(t *testing.T) {
	p := newTestPool(
		config.Account{ID: "a", Enabled: true},
		config.Account{ID: "b", Enabled: true},
		config.Account{ID: "c", Enabled: true},
	)
	scope := NewBoundScope([]string{"b"})
	for i := 0; i < 10; i++ {
		acc := p.GetNextExcludingScoped(nil, scope)
		if acc == nil {
			t.Fatal("expected bound account b, got nil")
		}
		if acc.ID != "b" {
			t.Fatalf("bound scope leaked account %q, expected only b", acc.ID)
		}
	}
}

func TestGetNextExcludingScopedSharedSkipsExclusive(t *testing.T) {
	p := newTestPool(
		config.Account{ID: "excl", Enabled: true},
		config.Account{ID: "shared", Enabled: true},
	)
	scope := NewSharedScope(map[string]bool{"excl": true})
	for i := 0; i < 10; i++ {
		acc := p.GetNextExcludingScoped(nil, scope)
		if acc == nil {
			t.Fatal("expected shared account, got nil")
		}
		if acc.ID == "excl" {
			t.Fatal("shared request must not route to an exclusive account")
		}
	}
}

func TestGetNextExcludingScopedBoundReturnsNilWhenBoundAccountMissing(t *testing.T) {
	p := newTestPool(
		config.Account{ID: "a", Enabled: true},
		config.Account{ID: "b", Enabled: true},
	)
	// Key is bound to an account that isn't in the pool (disabled/deleted).
	scope := NewBoundScope([]string{"ghost"})
	if acc := p.GetNextExcludingScoped(nil, scope); acc != nil {
		t.Fatalf("expected nil when no bound account is available, got %q", acc.ID)
	}
}

func TestGetNextExcludingScopedNilScopeBehavesLikeUnscoped(t *testing.T) {
	p := newTestPool(config.Account{ID: "only", Enabled: true})
	acc := p.GetNextExcludingScoped(nil, nil)
	if acc == nil || acc.ID != "only" {
		t.Fatalf("nil scope should allow the only account, got %#v", acc)
	}
}

// ---------------------------------------------------------------------------
// GetNextForModelExcludingScoped
// ---------------------------------------------------------------------------

func TestGetNextForModelExcludingScopedBoundRestricts(t *testing.T) {
	p := newTestPool(
		config.Account{ID: "a", Enabled: true},
		config.Account{ID: "b", Enabled: true},
	)
	p.SetModelList("a", []string{"claude-sonnet-4.5"})
	p.SetModelList("b", []string{"claude-sonnet-4.5"})

	scope := NewBoundScope([]string{"a"})
	for i := 0; i < 10; i++ {
		acc := p.GetNextForModelExcludingScoped("claude-sonnet-4.5", nil, scope)
		if acc == nil {
			t.Fatal("expected bound account a, got nil")
		}
		if acc.ID != "a" {
			t.Fatalf("bound scope leaked account %q, expected only a", acc.ID)
		}
	}
}

func TestGetNextForModelExcludingScopedSharedSkipsExclusive(t *testing.T) {
	p := newTestPool(
		config.Account{ID: "excl", Enabled: true},
		config.Account{ID: "shared", Enabled: true},
	)
	p.SetModelList("excl", []string{"claude-sonnet-4.5"})
	p.SetModelList("shared", []string{"claude-sonnet-4.5"})

	scope := NewSharedScope(map[string]bool{"excl": true})
	for i := 0; i < 10; i++ {
		acc := p.GetNextForModelExcludingScoped("claude-sonnet-4.5", nil, scope)
		if acc == nil {
			t.Fatal("expected shared account, got nil")
		}
		if acc.ID == "excl" {
			t.Fatal("shared request must not route to an exclusive account")
		}
	}
}

// scope and excluded must compose: an account allowed by scope but present in
// excluded is still skipped.
func TestGetNextForModelExcludingScopedComposesWithExcluded(t *testing.T) {
	p := newTestPool(
		config.Account{ID: "a", Enabled: true},
		config.Account{ID: "b", Enabled: true},
	)
	p.SetModelList("a", []string{"claude-sonnet-4.5"})
	p.SetModelList("b", []string{"claude-sonnet-4.5"})

	// Bound to both a and b, but a already failed (excluded) → only b eligible.
	scope := NewBoundScope([]string{"a", "b"})
	excluded := map[string]bool{"a": true}
	for i := 0; i < 10; i++ {
		acc := p.GetNextForModelExcludingScoped("claude-sonnet-4.5", excluded, scope)
		if acc == nil || acc.ID != "b" {
			t.Fatalf("expected only account b, got %#v", acc)
		}
	}
}
