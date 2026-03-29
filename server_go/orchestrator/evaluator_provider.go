package orchestrator

import (
	"strings"
)

const (
	EvaluatorProviderOpenAI           = "openai"
	EvaluatorProviderNVIDIA           = "nvidia"
	EvaluatorProviderOpenRouter       = "openrouter"
	EvaluatorProviderAnthropic        = "anthropic"
	EvaluatorProviderOpenAICompatible = "openai_compatible"
	EvaluatorProviderAuto             = "auto"
)

func NormalizeEvaluatorProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case EvaluatorProviderNVIDIA:
		return EvaluatorProviderNVIDIA
	case EvaluatorProviderOpenRouter:
		return EvaluatorProviderOpenRouter
	case EvaluatorProviderAnthropic, "claude":
		return EvaluatorProviderAnthropic
	case EvaluatorProviderOpenAICompatible, "compatible", "custom", "custom_openai":
		return EvaluatorProviderOpenAICompatible
	case EvaluatorProviderAuto:
		return EvaluatorProviderAuto
	case EvaluatorProviderOpenAI, "":
		return EvaluatorProviderOpenAI
	default:
		return EvaluatorProviderOpenAI
	}
}

func PreferredEvaluatorProvider(cfg map[string]any) string {
	return NormalizeEvaluatorProvider(firstNonEmptyString(cfg, "llm_provider", "evaluator_provider", "provider"))
}

func EvaluatorOpenAIAPIKey(cfg map[string]any) string {
	return strings.TrimSpace(firstNonEmptyString(cfg, "openai_api_key", "api_key"))
}

func EvaluatorNVIDIAAPIKey(cfg map[string]any) string {
	return strings.TrimSpace(firstNonEmptyString(cfg, "nvidia_api_key", "api_key"))
}

func EvaluatorOpenAIPromptID(cfg map[string]any) string {
	return strings.TrimSpace(firstNonEmptyString(cfg, "openai_prompt_id", "prompt_id"))
}

func EvaluatorOpenRouterAPIKey(cfg map[string]any) string {
	return strings.TrimSpace(firstNonEmptyString(cfg, "openrouter_api_key", "api_key"))
}

func EvaluatorCompatibleAPIKey(cfg map[string]any) string {
	return strings.TrimSpace(firstNonEmptyString(cfg, "compatible_api_key", "openai_compatible_api_key", "api_key"))
}

func EvaluatorCompatibleBaseURL(cfg map[string]any) string {
	return strings.TrimSpace(firstNonEmptyString(cfg, "compatible_base_url", "openai_compatible_base_url", "base_url"))
}

func EvaluatorAnthropicAPIKey(cfg map[string]any) string {
	return strings.TrimSpace(firstNonEmptyString(cfg, "anthropic_api_key", "api_key"))
}

func EvaluatorOpenAIMode(cfg map[string]any) string {
	mode := strings.ToLower(strings.TrimSpace(firstNonEmptyString(cfg, "openai_mode")))
	promptID := EvaluatorOpenAIPromptID(cfg)
	switch mode {
	case "managed", "managed_prompt", "prompt":
		return "managed"
	case "standard", "direct", "default":
		return "standard"
	}
	if promptID != "" {
		return "managed"
	}
	return "standard"
}

func IsEvaluatorOpenAIConfigured(cfg map[string]any) bool {
	if EvaluatorOpenAIAPIKey(cfg) == "" {
		return false
	}
	if EvaluatorOpenAIMode(cfg) == "managed" && EvaluatorOpenAIPromptID(cfg) == "" {
		return false
	}
	return true
}

func IsEvaluatorNVIDIAConfigured(cfg map[string]any) bool {
	return EvaluatorNVIDIAAPIKey(cfg) != ""
}

func IsEvaluatorOpenRouterConfigured(cfg map[string]any) bool {
	return EvaluatorOpenRouterAPIKey(cfg) != ""
}

func IsEvaluatorCompatibleConfigured(cfg map[string]any) bool {
	if EvaluatorCompatibleAPIKey(cfg) == "" {
		return false
	}
	return EvaluatorCompatibleBaseURL(cfg) != ""
}

func IsEvaluatorAnthropicConfigured(cfg map[string]any) bool {
	return EvaluatorAnthropicAPIKey(cfg) != ""
}

func HasConfiguredEvaluatorProvider(cfg map[string]any) bool {
	return IsEvaluatorOpenAIConfigured(cfg) ||
		IsEvaluatorNVIDIAConfigured(cfg) ||
		IsEvaluatorOpenRouterConfigured(cfg) ||
		IsEvaluatorCompatibleConfigured(cfg) ||
		IsEvaluatorAnthropicConfigured(cfg)
}

func isAutoEvaluatorNVIDIAConfigured(cfg map[string]any) bool {
	return strings.TrimSpace(firstNonEmptyString(cfg, "nvidia_api_key")) != ""
}

func isAutoEvaluatorOpenRouterConfigured(cfg map[string]any) bool {
	return strings.TrimSpace(firstNonEmptyString(cfg, "openrouter_api_key")) != ""
}

func isAutoEvaluatorAnthropicConfigured(cfg map[string]any) bool {
	return strings.TrimSpace(firstNonEmptyString(cfg, "anthropic_api_key")) != ""
}

func isAutoEvaluatorCompatibleConfigured(cfg map[string]any) bool {
	apiKey := strings.TrimSpace(firstNonEmptyString(cfg, "compatible_api_key", "openai_compatible_api_key"))
	if apiKey == "" {
		return false
	}
	return strings.TrimSpace(firstNonEmptyString(cfg, "compatible_base_url", "openai_compatible_base_url", "base_url")) != ""
}

func isAutoEvaluatorOpenAIConfigured(cfg map[string]any) bool {
	if strings.TrimSpace(firstNonEmptyString(cfg, "openai_api_key", "api_key")) == "" {
		return false
	}
	if EvaluatorOpenAIMode(cfg) == "managed" && EvaluatorOpenAIPromptID(cfg) == "" {
		return false
	}
	return true
}

func hasConfiguredAutoEvaluatorProvider(cfg map[string]any) bool {
	return isAutoEvaluatorOpenAIConfigured(cfg) ||
		isAutoEvaluatorNVIDIAConfigured(cfg) ||
		isAutoEvaluatorOpenRouterConfigured(cfg) ||
		isAutoEvaluatorCompatibleConfigured(cfg) ||
		isAutoEvaluatorAnthropicConfigured(cfg)
}

func ResolveEvaluatorProvider(cfg map[string]any) string {
	preferredProvider := PreferredEvaluatorProvider(cfg)

	if preferredProvider == EvaluatorProviderAuto {
		nvidiaConfigured := isAutoEvaluatorNVIDIAConfigured(cfg)
		openRouterConfigured := isAutoEvaluatorOpenRouterConfigured(cfg)
		anthropicConfigured := isAutoEvaluatorAnthropicConfigured(cfg)
		openaiConfigured := isAutoEvaluatorOpenAIConfigured(cfg)
		compatibleConfigured := isAutoEvaluatorCompatibleConfigured(cfg)

		order := evaluatorAutoProviderOrder(cfg)
		for _, provider := range order {
			switch provider {
			case EvaluatorProviderNVIDIA:
				if nvidiaConfigured {
					return EvaluatorProviderNVIDIA
				}
			case EvaluatorProviderOpenRouter:
				if openRouterConfigured {
					return EvaluatorProviderOpenRouter
				}
			case EvaluatorProviderAnthropic:
				if anthropicConfigured {
					return EvaluatorProviderAnthropic
				}
			case EvaluatorProviderOpenAI:
				if openaiConfigured {
					return EvaluatorProviderOpenAI
				}
			case EvaluatorProviderOpenAICompatible:
				if compatibleConfigured {
					return EvaluatorProviderOpenAICompatible
				}
			}
		}
		return EvaluatorProviderOpenAI
	}

	return preferredProvider
}

func IsSelectedEvaluatorProviderConfigured(cfg map[string]any) bool {
	switch PreferredEvaluatorProvider(cfg) {
	case EvaluatorProviderNVIDIA:
		return IsEvaluatorNVIDIAConfigured(cfg)
	case EvaluatorProviderOpenRouter:
		return IsEvaluatorOpenRouterConfigured(cfg)
	case EvaluatorProviderAnthropic:
		return IsEvaluatorAnthropicConfigured(cfg)
	case EvaluatorProviderOpenAICompatible:
		return IsEvaluatorCompatibleConfigured(cfg)
	case EvaluatorProviderAuto:
		return hasConfiguredAutoEvaluatorProvider(cfg)
	default:
		return IsEvaluatorOpenAIConfigured(cfg)
	}
}

func evaluatorAutoProviderOrder(cfg map[string]any) []string {
	priority := strings.ToLower(strings.TrimSpace(firstNonEmptyString(cfg, "provider_priority", "provider_preference", "provider_order")))
	if priority == "" {
		return []string{
			EvaluatorProviderNVIDIA,
			EvaluatorProviderOpenRouter,
			EvaluatorProviderAnthropic,
			EvaluatorProviderOpenAI,
			EvaluatorProviderOpenAICompatible,
		}
	}

	parts := strings.FieldsFunc(priority, func(r rune) bool {
		return r == ',' || r == ';' || r == '>' || r == '|' || r == ' '
	})
	seen := map[string]struct{}{}
	order := make([]string, 0, 4)
	for _, part := range parts {
		p := NormalizeEvaluatorProvider(part)
		if p == EvaluatorProviderAuto {
			continue
		}
		if _, exists := seen[p]; exists {
			continue
		}
		seen[p] = struct{}{}
		order = append(order, p)
	}

	defaultOrder := []string{
		EvaluatorProviderNVIDIA,
		EvaluatorProviderOpenRouter,
		EvaluatorProviderAnthropic,
		EvaluatorProviderOpenAI,
		EvaluatorProviderOpenAICompatible,
	}
	for _, p := range defaultOrder {
		if _, exists := seen[p]; exists {
			continue
		}
		order = append(order, p)
	}
	return order
}
