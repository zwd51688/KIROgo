package proxy

import (
	"kiro-go/config"
	"testing"
)

// routingScopeForApiKey resolves the per-request account routing scope from the
// matched API key ID. These tests exercise the three branches:
//   - bound key   → restricted to its bound accounts
//   - unbound key → shared scope (exclusive accounts excluded)
//   - empty key   → shared scope (exclusive accounts excluded)

func TestRoutingScopeForBoundKeyRestrictsToBoundAccounts(t *testing.T) {
	mustInitConfig(t)
	created, err := config.AddApiKey(config.ApiKeyEntry{
		Name:          "bound",
		Key:           "sk-bound",
		Enabled:       true,
		BoundAccounts: []string{"acc-1", "acc-2"},
	})
	if err != nil {
		t.Fatalf("seed bound key: %v", err)
	}

	scope := routingScopeForApiKey(created.ID)
	if scope == nil {
		t.Fatal("expected non-nil bound scope")
	}
	if !scope.Allows("acc-1") || !scope.Allows("acc-2") {
		t.Fatal("bound scope must allow its bound accounts")
	}
	if scope.Allows("acc-3") {
		t.Fatal("bound scope must reject accounts it is not bound to")
	}
}

func TestRoutingScopeForUnboundKeyExcludesExclusiveAccounts(t *testing.T) {
	mustInitConfig(t)
	// One key binds acc-excl (making it exclusive); the key under test has no bindings.
	if _, err := config.AddApiKey(config.ApiKeyEntry{
		Name:          "owner",
		Key:           "sk-owner",
		Enabled:       true,
		BoundAccounts: []string{"acc-excl"},
	}); err != nil {
		t.Fatalf("seed owner key: %v", err)
	}
	unbound, err := config.AddApiKey(config.ApiKeyEntry{Name: "free", Key: "sk-free", Enabled: true})
	if err != nil {
		t.Fatalf("seed unbound key: %v", err)
	}

	scope := routingScopeForApiKey(unbound.ID)
	if scope == nil {
		t.Fatal("expected non-nil shared scope when an exclusive account exists")
	}
	if scope.Allows("acc-excl") {
		t.Fatal("unbound key must not route to another key's exclusive account")
	}
	if !scope.Allows("acc-shared") {
		t.Fatal("unbound key must still route to shared accounts")
	}
}

func TestRoutingScopeForEmptyKeyUsesSharedScope(t *testing.T) {
	mustInitConfig(t)
	if _, err := config.AddApiKey(config.ApiKeyEntry{
		Name:          "owner",
		Key:           "sk-owner",
		Enabled:       true,
		BoundAccounts: []string{"acc-excl"},
	}); err != nil {
		t.Fatalf("seed owner key: %v", err)
	}

	// Empty apiKeyID models an unauthenticated request (RequireApiKey off).
	scope := routingScopeForApiKey("")
	if scope == nil {
		t.Fatal("expected non-nil shared scope")
	}
	if scope.Allows("acc-excl") {
		t.Fatal("unauthenticated request must not consume exclusive accounts")
	}
	if !scope.Allows("acc-shared") {
		t.Fatal("unauthenticated request must still use shared accounts")
	}
}

func TestRoutingScopeNoExclusiveAccountsReturnsNilScope(t *testing.T) {
	mustInitConfig(t)
	// A single unbound key, nothing exclusive → no restriction at all.
	unbound, err := config.AddApiKey(config.ApiKeyEntry{Name: "free", Key: "sk-free", Enabled: true})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	if scope := routingScopeForApiKey(unbound.ID); scope != nil {
		t.Fatal("expected nil scope (no restriction) when no account is exclusive")
	}
	if scope := routingScopeForApiKey(""); scope != nil {
		t.Fatal("expected nil scope (no restriction) for empty key when nothing is exclusive")
	}
}
