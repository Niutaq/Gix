package finops

import (
	"testing"
)

// TestGovernanceEngine_IsAllowed checks if the governance engine is allowed
func TestGovernanceEngine_IsAllowed(t *testing.T) {
	// 1. Mock or setup a test DB if needed, but for unit test we can mock the blocked map
	engine := NewGovernanceEngine(nil, 1.0)

	providerID := "test-provider"

	if !engine.IsAllowed(providerID) {
		t.Errorf("Expected provider %s to be allowed initially", providerID)
	}

	engine.mu.Lock()
	engine.blocked[providerID] = true
	engine.mu.Unlock()

	if engine.IsAllowed(providerID) {
		t.Errorf("Expected provider %s to be blocked", providerID)
	}
}

// Integration-like test for evaluateGuardrails would require a real DB.
func TestGovernanceEngine_GetBlockedList(t *testing.T) {
	engine := NewGovernanceEngine(nil, 1.0)
	engine.mu.Lock()
	engine.blocked["p1"] = true
	engine.blocked["p2"] = true
	engine.mu.Unlock()

	list := engine.GetBlockedList()
	if len(list) != 2 {
		t.Errorf("Expected 2 blocked providers, got %d", len(list))
	}
}
