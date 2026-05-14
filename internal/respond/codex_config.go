package respond

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type catalogModel struct {
	Slug                     string           `json:"slug"`
	DisplayName              string           `json:"display_name"`
	Priority                 int              `json:"priority"`
	DefaultReasoningLevel    *string          `json:"default_reasoning_level,omitempty"`
	SupportedReasoningLevels []reasoningLevel `json:"supported_reasoning_levels"`
	ShellType                string           `json:"shell_type"`
	Visibility               string           `json:"visibility"`
	SupportedInAPI           bool             `json:"supported_in_api"`
	InputModalities          []string         `json:"input_modalities"`
	ContextWindow            *int             `json:"context_window,omitempty"`
}

type reasoningLevel struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

type modelCatalog struct {
	Models []catalogModel `json:"models"`
}

func RunCodexConfig() error {
	// providerName := "respond"

	codexHome := os.Getenv("CODEX_HOME")
	var codexDir string
	if codexHome != "" {
		codexDir = codexHome
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		codexDir = filepath.Join(homeDir, ".codex")
	}
	codexConfigPath := filepath.Join(codexDir, "config.toml")
	// catalogPath := filepath.Join(codexDir, "catalog-respond.json")

	// var catalogPathAbs string
	// if len(config.Models) > 0 {
	// 	var err error
	// 	catalogPathAbs, err = writeCatalog(catalogPath, config.Models)
	// 	if err != nil {
	// 		return fmt.Errorf("cannot write model catalog: %w", err)
	// 	}
	// }

	backupPath := ""
	if _, err := os.Stat(codexConfigPath); err == nil {
		backupPath = codexConfigPath + ".bak." + time.Now().Format("20060102_150405")
		input, err := os.ReadFile(codexConfigPath)
		if err != nil {
			return fmt.Errorf("cannot read existing config for backup: %w", err)
		}
		if err := os.WriteFile(backupPath, input, 0644); err != nil {
			return fmt.Errorf("cannot create backup: %w", err)
		}
	}

	// var oldContent []byte
	// if _, err := os.Stat(codexConfigPath); err == nil {
	// 	oldContent, err = os.ReadFile(codexConfigPath)
	// 	if err != nil {
	// 		return fmt.Errorf("cannot read existing config: %w", err)
	// 	}
	// }

	// newContent := buildCodexConfig(string(oldContent), providerName, config.BaseURL(), config.APIKeyEnv, catalogPathAbs)

	// if err := os.MkdirAll(filepath.Dir(codexConfigPath), 0755); err != nil {
	// 	return fmt.Errorf("cannot create config directory: %w", err)
	// }
	// if err := os.WriteFile(codexConfigPath, []byte(newContent), 0644); err != nil {
	// 	return fmt.Errorf("cannot write config: %w", err)
	// }

	// fmt.Println("✓ Codex configuration updated")
	// fmt.Println()
	// fmt.Printf("  File:       %s\n", codexConfigPath)
	// if backupPath != "" {
	// 	fmt.Printf("  Backup:     %s\n", backupPath)
	// }
	// fmt.Printf("  Catalog:    %s\n", catalogPath)
	// fmt.Println()
	// fmt.Println("  Changes made:")
	// fmt.Printf("    model_provider        = %s\n", providerName)
	// fmt.Printf("    model_catalog_json    = %s\n", catalogPathAbs)
	// fmt.Printf("    [model_providers.%s]\n", providerName)
	// fmt.Printf("      name                = %s\n", providerName)
	// fmt.Printf("      base_url            = %s\n", config.BaseURL())
	// if config.APIKeyEnv != "" {
	// 	fmt.Printf("      env_key             = %s\n", config.APIKeyEnv)
	// }
	// fmt.Printf("      wire_api            = responses\n")
	// fmt.Println()
	// fmt.Printf("  Models (%d):\n", len(config.Models))
	// for _, m := range config.Models {
	// 	fmt.Printf("    - %s (priority %d)\n", m.Slug, m.Priority)
	// }
	// fmt.Println()
	// if backupPath != "" {
	// 	fmt.Println("  Tip: Restore previous config with:")
	// 	fmt.Printf("    cp %s %s\n", backupPath, codexConfigPath)
	// }

	return nil
}

func writeCatalog(path string, entries []Model) (string, error) {
	models := make([]catalogModel, 0, len(entries))
	// for _, e := range entries {
	// 	cm := catalogModel{
	// 		Slug:            e.Slug,
	// 		DisplayName:     e.DisplayName,
	// 		Priority:        e.Priority,
	// 		ShellType:       "shell_command",
	// 		Visibility:      "list",
	// 		SupportedInAPI:  true,
	// 		InputModalities: e.InputModalities,
	// 	}
	// 	if e.ContextWindow > 0 {
	// 		cm.ContextWindow = &e.ContextWindow
	// 	}
	// 	if e.DefaultReasoningEffort != "" {
	// 		val := e.DefaultReasoningEffort
	// 		cm.DefaultReasoningLevel = &val
	// 	}
	// 	cm.SupportedReasoningLevels = buildReasoningLevels(e.DefaultReasoningEffort)

	// 	models = append(models, cm)
	// }

	catalog := modelCatalog{Models: models}
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return absPath, nil
}

func buildReasoningLevels(defaultEffort string) []reasoningLevel {
	all := []reasoningLevel{
		{Effort: "low", Description: "Fast responses with lighter reasoning"},
		{Effort: "medium", Description: "Balances speed and reasoning depth for everyday tasks"},
		{Effort: "high", Description: "Greater reasoning depth for complex problems"},
	}

	if defaultEffort == "" {
		return all
	}

	if defaultEffort == "high" {
		all = append(all, reasoningLevel{
			Effort: "xhigh", Description: "Extra high reasoning depth for complex problems",
		})
	}
	return all
}

func buildCodexConfig(existing, providerName, baseURL, apiKeyEnv, catalogPath string) string {
	if existing == "" {
		return newCodexConfig(providerName, baseURL, apiKeyEnv, catalogPath)
	}

	lines := strings.Split(existing, "\n")
	var result []string

	type sectionState int
	const (
		stateTop sectionState = iota
		stateOtherSection
		stateModelProviders
	)

	state := stateTop
	inProviderSection := false
	wroteProviderKey := false
	wroteCatalogKey := false
	wroteProviderSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "[[") && strings.HasSuffix(trimmed, "]") {
			inProviderSection = false
			wroteProviderSection = false

			section := trimmed[1 : len(trimmed)-1]
			if section == "model_providers" || strings.HasPrefix(section, "model_providers.") {
				state = stateModelProviders
				if section == "model_providers."+providerName {
					inProviderSection = true
				}
			} else {
				state = stateOtherSection
			}

			if state != stateTop && !wroteProviderKey {
				result = append(result, fmt.Sprintf(`model_provider = "%s"`, providerName))
				wroteProviderKey = true
			}
			if state != stateTop && !wroteCatalogKey && catalogPath != "" {
				result = append(result, fmt.Sprintf(`model_catalog_json = "%s"`, catalogPath))
				wroteCatalogKey = true
			}

			result = append(result, line)
			continue
		}

		if state == stateTop || state == stateOtherSection {
			if isKeyLine(trimmed, "model_provider", "=") {
				result = append(result, fmt.Sprintf(`model_provider = "%s"`, providerName))
				wroteProviderKey = true
				continue
			}
			if isKeyLine(trimmed, "model_catalog_json", "=") {
				if catalogPath != "" {
					result = append(result, fmt.Sprintf(`model_catalog_json = "%s"`, catalogPath))
				}
				wroteCatalogKey = true
				continue
			}
			if isKeyLine(trimmed, "model", "=") && !isKeyLine(trimmed, "model_", "=") {
				continue
			}
			if isKeyLine(trimmed, "model_reasoning_effort", "=") {
				continue
			}

			if strings.HasPrefix(trimmed, "#") {
				commentContent := strings.TrimSpace(trimmed[1:])
				skip := false
				switch {
				case isKeyLine(commentContent, "model_provider", "="):
					if !wroteProviderKey {
						result = append(result, fmt.Sprintf(`model_provider = "%s"`, providerName))
						wroteProviderKey = true
					}
					skip = true
				case isKeyLine(commentContent, "model_catalog_json", "="):
					if !wroteCatalogKey && catalogPath != "" {
						result = append(result, fmt.Sprintf(`model_catalog_json = "%s"`, catalogPath))
						wroteCatalogKey = true
					}
					skip = true
				case isKeyLine(commentContent, "model", "=") && !isKeyLine(commentContent, "model_", "="):
					skip = true
				case isKeyLine(commentContent, "model_reasoning_effort", "="):
					skip = true
				}
				if skip {
					continue
				}
			}
		}

		if state == stateModelProviders && inProviderSection {
			if isKeyLine(trimmed, "name", "=") {
				if !wroteProviderSection {
					result = append(result, fmt.Sprintf(`name = "%s"`, providerName))
					wroteProviderSection = true
				}
				continue
			}
			if isKeyLine(trimmed, "base_url", "=") {
				result = append(result, fmt.Sprintf(`base_url = "%s"`, baseURL))
				continue
			}
			if isKeyLine(trimmed, "env_key", "=") {
				if apiKeyEnv != "" {
					result = append(result, fmt.Sprintf(`env_key = "%s"`, apiKeyEnv))
				}
				continue
			}
			if isKeyLine(trimmed, "wire_api", "=") {
				result = append(result, `wire_api = "responses"`)
				continue
			}
		}

		result = append(result, line)
	}

	if !wroteProviderKey {
		result = append(result, fmt.Sprintf(`model_provider = "%s"`, providerName))
	}
	if !wroteCatalogKey && catalogPath != "" {
		result = append(result, fmt.Sprintf(`model_catalog_json = "%s"`, catalogPath))
	}

	hasProviderSection := false
	for _, line := range result {
		if strings.TrimSpace(line) == fmt.Sprintf(`[model_providers.%s]`, providerName) {
			hasProviderSection = true
			break
		}
	}

	if !hasProviderSection {
		result = append(result, "")
		result = append(result, fmt.Sprintf(`[model_providers.%s]`, providerName))
		result = append(result, fmt.Sprintf(`name = "%s"`, providerName))
		result = append(result, fmt.Sprintf(`base_url = "%s"`, baseURL))
		if apiKeyEnv != "" {
			result = append(result, fmt.Sprintf(`env_key = "%s"`, apiKeyEnv))
		}
		result = append(result, `wire_api = "responses"`)
	}

	return strings.Join(result, "\n")
}

func isKeyLine(trimmed, key, sep string) bool {
	before, _, _ := strings.Cut(trimmed, sep)
	return strings.TrimSpace(before) == key
}

func newCodexConfig(providerName, baseURL, apiKeyEnv, catalogPath string) string {
	var b strings.Builder

	fmt.Fprintf(&b, `model_provider = "%s"`, providerName)
	b.WriteString("\n")
	if catalogPath != "" {
		fmt.Fprintf(&b, `model_catalog_json = "%s"`, catalogPath)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "[model_providers.%s]\n", providerName)
	fmt.Fprintf(&b, `name = "%s"`, providerName)
	b.WriteString("\n")
	fmt.Fprintf(&b, `base_url = "%s"`, baseURL)
	b.WriteString("\n")
	if apiKeyEnv != "" {
		fmt.Fprintf(&b, `env_key = "%s"`, apiKeyEnv)
		b.WriteString("\n")
	}
	b.WriteString(`wire_api = "responses"`)
	b.WriteString("\n")

	return b.String()
}
