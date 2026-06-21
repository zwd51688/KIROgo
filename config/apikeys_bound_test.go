package config

import (
	"path/filepath"
	"testing"
)

// GetBoundAccountIDs returns the bound IDs for a key, or nil when the key has
// no bindings or does not exist.
func TestGetBoundAccountIDs(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "config.json")
	if err := Init(cfgFile); err != nil {
		t.Fatalf("init: %v", err)
	}

	bound, err := AddApiKey(ApiKeyEntry{
		Name:          "bound",
		Key:           "sk-bound",
		Enabled:       true,
		BoundAccounts: []string{"acc-1", "acc-2"},
	})
	if err != nil {
		t.Fatalf("add bound: %v", err)
	}
	unbound, err := AddApiKey(ApiKeyEntry{Name: "unbound", Key: "sk-unbound", Enabled: true})
	if err != nil {
		t.Fatalf("add unbound: %v", err)
	}

	ids := GetBoundAccountIDs(bound.ID)
	if len(ids) != 2 {
		t.Fatalf("expected 2 bound IDs, got %d (%v)", len(ids), ids)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if !got["acc-1"] || !got["acc-2"] {
		t.Fatalf("expected acc-1 and acc-2, got %v", ids)
	}

	if ids := GetBoundAccountIDs(unbound.ID); ids != nil {
		t.Fatalf("expected nil for unbound key, got %v", ids)
	}
	if ids := GetBoundAccountIDs("does-not-exist"); ids != nil {
		t.Fatalf("expected nil for unknown key, got %v", ids)
	}
}

// GetBoundAccountIDs must return a copy: mutating it must not corrupt config state.
func TestGetBoundAccountIDsReturnsCopy(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "config.json")
	if err := Init(cfgFile); err != nil {
		t.Fatalf("init: %v", err)
	}
	created, err := AddApiKey(ApiKeyEntry{
		Name:          "bound",
		Key:           "sk-bound",
		Enabled:       true,
		BoundAccounts: []string{"acc-1"},
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	ids := GetBoundAccountIDs(created.ID)
	ids[0] = "tampered"

	fresh := GetBoundAccountIDs(created.ID)
	if len(fresh) != 1 || fresh[0] != "acc-1" {
		t.Fatalf("mutating returned slice leaked into config: %v", fresh)
	}
}

// GetExclusiveAccountIDs aggregates bound accounts across all keys.
func TestGetExclusiveAccountIDs(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "config.json")
	if err := Init(cfgFile); err != nil {
		t.Fatalf("init: %v", err)
	}

	if _, err := AddApiKey(ApiKeyEntry{Name: "k1", Key: "sk-1", Enabled: true, BoundAccounts: []string{"a", "b"}}); err != nil {
		t.Fatalf("add k1: %v", err)
	}
	if _, err := AddApiKey(ApiKeyEntry{Name: "k2", Key: "sk-2", Enabled: true, BoundAccounts: []string{"b", "c"}}); err != nil {
		t.Fatalf("add k2: %v", err)
	}
	if _, err := AddApiKey(ApiKeyEntry{Name: "k3", Key: "sk-3", Enabled: true}); err != nil {
		t.Fatalf("add k3: %v", err)
	}

	excl := GetExclusiveAccountIDs()
	for _, id := range []string{"a", "b", "c"} {
		if !excl[id] {
			t.Fatalf("expected %q to be exclusive, got %v", id, excl)
		}
	}
	if len(excl) != 3 {
		t.Fatalf("expected exactly 3 exclusive accounts (a,b,c), got %v", excl)
	}
	if excl["shared"] {
		t.Fatalf("unexpected exclusive account 'shared'")
	}
}

func TestGetExclusiveAccountIDsEmptyWhenNoBindings(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "config.json")
	if err := Init(cfgFile); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := AddApiKey(ApiKeyEntry{Name: "k", Key: "sk-k", Enabled: true}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if excl := GetExclusiveAccountIDs(); len(excl) != 0 {
		t.Fatalf("expected no exclusive accounts, got %v", excl)
	}
}

// BoundAccounts must survive a save/reload round-trip, and the update path must
// normalize (trim/dedupe/drop-empty) the bindings.
func TestBoundAccountsPersistAndNormalize(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "config.json")
	if err := Init(cfgFile); err != nil {
		t.Fatalf("init: %v", err)
	}
	created, err := AddApiKey(ApiKeyEntry{Name: "k", Key: "sk-k", Enabled: true})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	// Patch with messy bindings: duplicates, blanks, surrounding whitespace.
	patch := *GetApiKeyEntry(created.ID)
	patch.BoundAccounts = []string{"acc-1", " acc-1 ", "", "acc-2", "  "}
	if err := UpdateApiKey(created.ID, patch); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Reload from disk to confirm persistence.
	if err := Init(cfgFile); err != nil {
		t.Fatalf("re-init: %v", err)
	}
	got := GetApiKeyEntry(created.ID)
	if got == nil {
		t.Fatalf("entry missing after reload")
	}
	if len(got.BoundAccounts) != 2 {
		t.Fatalf("expected normalized bindings [acc-1 acc-2], got %v", got.BoundAccounts)
	}
	set := map[string]bool{}
	for _, id := range got.BoundAccounts {
		set[id] = true
	}
	if !set["acc-1"] || !set["acc-2"] {
		t.Fatalf("expected acc-1 and acc-2 after normalization, got %v", got.BoundAccounts)
	}
}

// Clearing BoundAccounts (empty slice in patch) makes the accounts shared again.
func TestBoundAccountsCanBeCleared(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "config.json")
	if err := Init(cfgFile); err != nil {
		t.Fatalf("init: %v", err)
	}
	created, err := AddApiKey(ApiKeyEntry{
		Name:          "k",
		Key:           "sk-k",
		Enabled:       true,
		BoundAccounts: []string{"acc-1"},
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	patch := *GetApiKeyEntry(created.ID)
	patch.BoundAccounts = []string{}
	if err := UpdateApiKey(created.ID, patch); err != nil {
		t.Fatalf("update: %v", err)
	}

	if ids := GetBoundAccountIDs(created.ID); ids != nil {
		t.Fatalf("expected bindings cleared, got %v", ids)
	}
	if excl := GetExclusiveAccountIDs(); len(excl) != 0 {
		t.Fatalf("expected account to become shared, got %v", excl)
	}
}
