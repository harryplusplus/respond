package goblin

import _ "embed"

// Source: https://github.com/openai/codex/blob/main/codex-rs/models-manager/prompt.md (Apache-2.0)
//
//go:embed base_instructions.md
var baseInstructions string

type ReasoningEffort string

const (
	ReasoningEffortNone      ReasoningEffort = "none"
	ReasoningEffortMinimal   ReasoningEffort = "minimal"
	ReasoningEffortLow       ReasoningEffort = "low"
	ReasoningEffortMedium    ReasoningEffort = "medium"
	ReasoningEffortHigh      ReasoningEffort = "high"
	ReasoningEffortExtraHigh ReasoningEffort = "xhigh"
)

type ConfigShellToolType string

const (
	ShellToolTypeDefault      ConfigShellToolType = "default"
	ShellToolTypeLocal        ConfigShellToolType = "local"
	ShellToolTypeUnifiedExec  ConfigShellToolType = "unified_exec"
	ShellToolTypeDisabled     ConfigShellToolType = "disabled"
	ShellToolTypeShellCommand ConfigShellToolType = "shell_command"
)

type ModelVisibility string

const (
	ModelVisibilityList ModelVisibility = "list"
	ModelVisibilityHide ModelVisibility = "hide"
	ModelVisibilityNone ModelVisibility = "none"
)

type TruncationMode string

const (
	TruncationModeBytes  TruncationMode = "bytes"
	TruncationModeTokens TruncationMode = "tokens"
)

type TruncationPolicyConfig struct {
	Mode  TruncationMode `yaml:"mode"  json:"mode"`
	Limit int            `yaml:"limit" json:"limit"`
}

type ReasoningEffortPreset struct {
	Effort      ReasoningEffort `yaml:"effort"      json:"effort"`
	Description string          `yaml:"description" json:"description"`
}

type ModelInfo struct {
	Slug                       string                  `yaml:"-"                            json:"slug"`
	DisplayName                string                  `yaml:"display_name"                 json:"display_name"`
	SupportedReasoningLevels   []ReasoningEffortPreset `yaml:"supported_reasoning_levels"   json:"supported_reasoning_levels"`
	ShellType                  ConfigShellToolType     `yaml:"shell_type"                   json:"shell_type"`
	Visibility                 ModelVisibility         `yaml:"visibility"                   json:"visibility"`
	SupportedInAPI             *bool                   `yaml:"supported_in_api"             json:"supported_in_api"`
	Priority                   *int                    `yaml:"priority"                     json:"priority"`
	BaseInstructions           *string                 `yaml:"base_instructions"            json:"base_instructions"`
	SupportsReasoningSummaries *bool                   `yaml:"supports_reasoning_summaries" json:"supports_reasoning_summaries"`
	SupportVerbosity           bool                    `yaml:"support_verbosity"            json:"support_verbosity"`
	TruncationPolicy           TruncationPolicyConfig  `yaml:"truncation_policy"            json:"truncation_policy"`
	SupportsParallelToolCalls  *bool                   `yaml:"supports_parallel_tool_calls" json:"supports_parallel_tool_calls"`
	ExperimentalSupportedTools []any                   `yaml:"experimental_supported_tools" json:"experimental_supported_tools"`
}

type ModelsResponse struct {
	Models []ModelInfo `json:"models"`
}

const defaultTruncationLimit = 10000

func fillModelDefaults(m *ModelInfo) {
	if m.DisplayName == "" {
		m.DisplayName = m.Slug
	}

	if m.ShellType == "" {
		m.ShellType = ShellToolTypeDefault
	}

	if m.Visibility == "" {
		m.Visibility = ModelVisibilityList
	}

	if m.Priority == nil {
		m.Priority = new(1)
	}

	if m.BaseInstructions == nil {
		m.BaseInstructions = new(baseInstructions)
	}

	if m.TruncationPolicy.Mode == "" {
		m.TruncationPolicy.Mode = TruncationModeTokens
	}

	if m.TruncationPolicy.Limit == 0 {
		m.TruncationPolicy.Limit = defaultTruncationLimit
	}

	if m.SupportedInAPI == nil {
		m.SupportedInAPI = new(true)
	}

	if m.SupportsReasoningSummaries == nil {
		m.SupportsReasoningSummaries = new(true)
	}

	if m.SupportsParallelToolCalls == nil {
		m.SupportsParallelToolCalls = new(true)
	}

	if m.ExperimentalSupportedTools == nil {
		m.ExperimentalSupportedTools = []any{}
	}

	for i := range m.SupportedReasoningLevels {
		if m.SupportedReasoningLevels[i].Description == "" {
			m.SupportedReasoningLevels[i].Description = string(m.SupportedReasoningLevels[i].Effort)
		}
	}
}
