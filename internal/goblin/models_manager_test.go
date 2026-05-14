package goblin

import (
	"reflect"
	"testing"
)

func hydrateTestConfig(cfg *Config) *Config {
	hydrateModels(cfg)
	return cfg
}

func TestNewModelsManager_EmptyProviders(t *testing.T) {
	mgr := NewModelsManager(hydrateTestConfig(&Config{}))
	resp := mgr.Models()
	if len(resp.Models) != 0 {
		t.Errorf("Models() = %d models, want 0", len(resp.Models))
	}
}

func TestNewModelsManager_DefaultsApplied(t *testing.T) {
	mgr := NewModelsManager(hydrateTestConfig(&Config{
		Providers: map[string]Provider{
			"test": {
				Models: map[string]*ModelInfo{
					"my-model": {
						Priority: new(10),
					},
				},
			},
		},
	}))

	resp := mgr.Models()
	if len(resp.Models) != 1 {
		t.Fatalf("Models() = %d models, want 1", len(resp.Models))
	}

	m := resp.Models[0]

	if m.Slug != "test/my-model" {
		t.Errorf("Slug = %q, want %q", m.Slug, "test/my-model")
	}

	if m.DisplayName != "test/my-model" {
		t.Errorf("DisplayName = %q, want %q (default from slug)", m.DisplayName, "test/my-model")
	}

	if m.ShellType != ShellToolTypeDefault {
		t.Errorf("ShellType = %q, want %q", m.ShellType, ShellToolTypeDefault)
	}

	if m.Visibility != ModelVisibilityList {
		t.Errorf("Visibility = %q, want %q", m.Visibility, ModelVisibilityList)
	}

	if m.SupportedInAPI == nil || *m.SupportedInAPI != true {
		t.Errorf("SupportedInAPI = %v, want true", m.SupportedInAPI)
	}

	if m.Priority == nil || *m.Priority != 10 {
		t.Errorf("Priority = %v, want 10", m.Priority)
	}

	if m.BaseInstructions == nil || *m.BaseInstructions == "" {
		t.Errorf("BaseInstructions = %v, want non-empty default", m.BaseInstructions)
	}

	if m.SupportsReasoningSummaries == nil || *m.SupportsReasoningSummaries != true {
		t.Errorf("SupportsReasoningSummaries = %v, want true", m.SupportsReasoningSummaries)
	}

	if m.SupportVerbosity {
		t.Errorf("SupportVerbosity = true, want false")
	}

	if m.TruncationPolicy.Mode != TruncationModeTokens {
		t.Errorf("TruncationPolicy.Mode = %q, want %q", m.TruncationPolicy.Mode, TruncationModeTokens)
	}

	if m.TruncationPolicy.Limit != 10000 {
		t.Errorf("TruncationPolicy.Limit = %d, want 10000", m.TruncationPolicy.Limit)
	}

	if m.SupportsParallelToolCalls == nil || *m.SupportsParallelToolCalls != true {
		t.Errorf("SupportsParallelToolCalls = %v, want true", m.SupportsParallelToolCalls)
	}
}

func TestNewModelsManager_SortedByPriorityAsc(t *testing.T) {
	mgr := NewModelsManager(hydrateTestConfig(&Config{
		Providers: map[string]Provider{
			"a": {
				Models: map[string]*ModelInfo{
					"low":  {Priority: new(1)},
					"high": {Priority: new(100)},
				},
			},
			"b": {
				Models: map[string]*ModelInfo{
					"mid": {Priority: new(50)},
				},
			},
		},
	}))

	resp := mgr.Models()
	if len(resp.Models) != 3 {
		t.Fatalf("Models() = %d models, want 3", len(resp.Models))
	}

	wantOrder := []string{"a/low", "b/mid", "a/high"}
	for i, m := range resp.Models {
		if m.Slug != wantOrder[i] {
			t.Errorf("Models[%d].Slug = %q, want %q", i, m.Slug, wantOrder[i])
		}
	}
}

func TestNewModelsManager_ExplicitFieldsPreserved(t *testing.T) {
	falseVal := false
	mgr := NewModelsManager(hydrateTestConfig(&Config{
		Providers: map[string]Provider{
			"test": {
				Models: map[string]*ModelInfo{
					"explicit-model": {
						DisplayName:                "My Explicit Model",
						ShellType:                  ShellToolTypeLocal,
						Visibility:                 ModelVisibilityHide,
						SupportedInAPI:             &falseVal,
						Priority:                   new(42),
						SupportsReasoningSummaries: &falseVal,
						SupportVerbosity:           true,
						TruncationPolicy: TruncationPolicyConfig{
							Mode:  TruncationModeBytes,
							Limit: 5000,
						},
						SupportsParallelToolCalls: &falseVal,
					},
				},
			},
		},
	}))

	resp := mgr.Models()
	if len(resp.Models) != 1 {
		t.Fatalf("Models() = %d models, want 1", len(resp.Models))
	}

	m := resp.Models[0]

	if m.DisplayName != "My Explicit Model" {
		t.Errorf("DisplayName = %q, want %q", m.DisplayName, "My Explicit Model")
	}

	if m.ShellType != ShellToolTypeLocal {
		t.Errorf("ShellType = %q, want %q", m.ShellType, ShellToolTypeLocal)
	}

	if m.Visibility != ModelVisibilityHide {
		t.Errorf("Visibility = %q, want %q", m.Visibility, ModelVisibilityHide)
	}

	if m.SupportedInAPI == nil || *m.SupportedInAPI != false {
		t.Errorf("SupportedInAPI = %v, want false", m.SupportedInAPI)
	}

	if m.Priority == nil || *m.Priority != 42 {
		t.Errorf("Priority = %v, want 42", m.Priority)
	}

	if m.SupportsReasoningSummaries == nil || *m.SupportsReasoningSummaries != false {
		t.Errorf("SupportsReasoningSummaries = %v, want false", m.SupportsReasoningSummaries)
	}

	if !m.SupportVerbosity {
		t.Errorf("SupportVerbosity = false, want true")
	}

	if m.TruncationPolicy.Mode != TruncationModeBytes {
		t.Errorf("TruncationPolicy.Mode = %q, want %q", m.TruncationPolicy.Mode, TruncationModeBytes)
	}

	if m.TruncationPolicy.Limit != 5000 {
		t.Errorf("TruncationPolicy.Limit = %d, want 5000", m.TruncationPolicy.Limit)
	}

	if m.SupportsParallelToolCalls == nil || *m.SupportsParallelToolCalls != false {
		t.Errorf("SupportsParallelToolCalls = %v, want false", m.SupportsParallelToolCalls)
	}
}

func TestNewModelsManager_ReasoningLevels(t *testing.T) {
	mgr := NewModelsManager(hydrateTestConfig(&Config{
		Providers: map[string]Provider{
			"test": {
				Models: map[string]*ModelInfo{
					"reasoning-model": {
						Priority: new(1),
						SupportedReasoningLevels: []ReasoningEffortPreset{
							{Effort: ReasoningEffortNone},
							{Effort: ReasoningEffortHigh},
						},
					},
				},
			},
		},
	}))

	resp := mgr.Models()
	if len(resp.Models) != 1 {
		t.Fatalf("Models() = %d models, want 1", len(resp.Models))
	}

	m := resp.Models[0]

	if len(m.SupportedReasoningLevels) != 2 {
		t.Fatalf("SupportedReasoningLevels = %d, want 2", len(m.SupportedReasoningLevels))
	}

	if m.SupportedReasoningLevels[0].Effort != ReasoningEffortNone {
		t.Errorf("SupportedReasoningLevels[0].Effort = %q, want %q", m.SupportedReasoningLevels[0].Effort, ReasoningEffortNone)
	}

	if m.SupportedReasoningLevels[1].Effort != ReasoningEffortHigh {
		t.Errorf("SupportedReasoningLevels[1].Effort = %q, want %q", m.SupportedReasoningLevels[1].Effort, ReasoningEffortHigh)
	}
}

func TestModelsResponse_JSONSerialization(t *testing.T) {
	mgr := NewModelsManager(hydrateTestConfig(&Config{
		Providers: map[string]Provider{
			"test": {
				Models: map[string]*ModelInfo{
					"test-model": {Priority: new(1)},
				},
			},
		},
	}))

	resp := mgr.Models()
	modelsField := reflect.ValueOf(resp).FieldByName("Models")
	if !modelsField.IsValid() {
		t.Fatal("ModelsResponse has no Models field")
	}

	if modelsField.Len() != 1 {
		t.Errorf("Models field length = %d, want 1", modelsField.Len())
	}
}
