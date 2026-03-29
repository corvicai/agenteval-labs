package orchestrator

import "testing"

func TestResolveEvaluatorProvider_DefaultOpenAI(t *testing.T) {
	provider := ResolveEvaluatorProvider(map[string]any{})
	if provider != EvaluatorProviderOpenAI {
		t.Fatalf("expected %s, got %s", EvaluatorProviderOpenAI, provider)
	}
}

func TestResolveEvaluatorProvider_ExplicitNVIDIA(t *testing.T) {
	provider := ResolveEvaluatorProvider(map[string]any{
		"llm_provider":   "nvidia",
		"nvidia_api_key": "nvapi-123",
	})
	if provider != EvaluatorProviderNVIDIA {
		t.Fatalf("expected %s, got %s", EvaluatorProviderNVIDIA, provider)
	}
}

func TestResolveEvaluatorProvider_ExplicitOpenRouter(t *testing.T) {
	provider := ResolveEvaluatorProvider(map[string]any{
		"llm_provider":       "openrouter",
		"openrouter_api_key": "or-123",
	})
	if provider != EvaluatorProviderOpenRouter {
		t.Fatalf("expected %s, got %s", EvaluatorProviderOpenRouter, provider)
	}
}

func TestResolveEvaluatorProvider_ExplicitOpenAICompatible(t *testing.T) {
	provider := ResolveEvaluatorProvider(map[string]any{
		"llm_provider":        "openai_compatible",
		"compatible_api_key":  "cmp-123",
		"compatible_base_url": "https://example.ai/v1",
	})
	if provider != EvaluatorProviderOpenAICompatible {
		t.Fatalf("expected %s, got %s", EvaluatorProviderOpenAICompatible, provider)
	}
}

func TestResolveEvaluatorProvider_AutoDefaultOrderPrefersNVIDIA(t *testing.T) {
	provider := ResolveEvaluatorProvider(map[string]any{
		"llm_provider":       "auto",
		"openai_api_key":     "sk-openai",
		"nvidia_api_key":     "nvapi-123",
		"openrouter_api_key": "or-123",
		"openai_mode":        "standard",
	})
	if provider != EvaluatorProviderNVIDIA {
		t.Fatalf("expected %s, got %s", EvaluatorProviderNVIDIA, provider)
	}
}

func TestResolveEvaluatorProvider_AutoCanPreferOpenAIByPriority(t *testing.T) {
	provider := ResolveEvaluatorProvider(map[string]any{
		"llm_provider":       "auto",
		"provider_priority":  "openai,nvidia,openrouter",
		"openai_api_key":     "sk-openai",
		"nvidia_api_key":     "nvapi-123",
		"openrouter_api_key": "or-123",
		"openai_mode":        "standard",
	})
	if provider != EvaluatorProviderOpenAI {
		t.Fatalf("expected %s, got %s", EvaluatorProviderOpenAI, provider)
	}
}

func TestResolveEvaluatorProvider_AutoFallsBackToOpenRouter(t *testing.T) {
	provider := ResolveEvaluatorProvider(map[string]any{
		"llm_provider":       "auto",
		"openrouter_api_key": "or-123",
	})
	if provider != EvaluatorProviderOpenRouter {
		t.Fatalf("expected %s, got %s", EvaluatorProviderOpenRouter, provider)
	}
}

func TestResolveEvaluatorProvider_AutoFallsBackToOpenAICompatible(t *testing.T) {
	provider := ResolveEvaluatorProvider(map[string]any{
		"llm_provider":        "auto",
		"compatible_api_key":  "cmp-123",
		"compatible_base_url": "https://example.ai/v1",
	})
	if provider != EvaluatorProviderOpenAICompatible {
		t.Fatalf("expected %s, got %s", EvaluatorProviderOpenAICompatible, provider)
	}
}

func TestResolveEvaluatorProvider_DoesNotFallbackWhenProviderExplicit(t *testing.T) {
	provider := ResolveEvaluatorProvider(map[string]any{
		"llm_provider":   "openai",
		"nvidia_api_key": "nvapi-123",
	})
	if provider != EvaluatorProviderOpenAI {
		t.Fatalf("expected explicit provider %s, got %s", EvaluatorProviderOpenAI, provider)
	}
}

func TestResolveEvaluatorProvider_DoesNotFallbackFromOpenRouter(t *testing.T) {
	provider := ResolveEvaluatorProvider(map[string]any{
		"llm_provider":   "openrouter",
		"openai_api_key": "sk-openai",
		"openai_mode":    "standard",
	})
	if provider != EvaluatorProviderOpenRouter {
		t.Fatalf("expected explicit provider %s, got %s", EvaluatorProviderOpenRouter, provider)
	}
}

func TestResolveEvaluatorProvider_ExplicitAnthropic(t *testing.T) {
	provider := ResolveEvaluatorProvider(map[string]any{
		"llm_provider":      "anthropic",
		"anthropic_api_key": "ak-123",
	})
	if provider != EvaluatorProviderAnthropic {
		t.Fatalf("expected %s, got %s", EvaluatorProviderAnthropic, provider)
	}
}

func TestIsEvaluatorOpenAIConfigured_ManagedRequiresPrompt(t *testing.T) {
	cfg := map[string]any{
		"openai_api_key": "sk-openai",
		"openai_mode":    "managed",
	}
	if IsEvaluatorOpenAIConfigured(cfg) {
		t.Fatalf("expected managed config without prompt to be invalid")
	}

	cfg["openai_prompt_id"] = "prompt_123"
	if !IsEvaluatorOpenAIConfigured(cfg) {
		t.Fatalf("expected managed config with prompt to be valid")
	}
}

func TestIsEvaluatorOpenRouterConfigured(t *testing.T) {
	if IsEvaluatorOpenRouterConfigured(map[string]any{}) {
		t.Fatalf("expected empty config to be invalid")
	}
	if !IsEvaluatorOpenRouterConfigured(map[string]any{"openrouter_api_key": "or-123"}) {
		t.Fatalf("expected openrouter config with api key to be valid")
	}
}

func TestIsEvaluatorCompatibleConfigured(t *testing.T) {
	if IsEvaluatorCompatibleConfigured(map[string]any{
		"compatible_api_key": "cmp-123",
	}) {
		t.Fatalf("expected missing base URL to be invalid")
	}
	if !IsEvaluatorCompatibleConfigured(map[string]any{
		"compatible_api_key":  "cmp-123",
		"compatible_base_url": "https://example.ai/v1",
	}) {
		t.Fatalf("expected compatible config to be valid")
	}
}

func TestEvaluatorLegacyAPIKeyCompatibility(t *testing.T) {
	cfg := map[string]any{
		"api_key":     "sk-legacy",
		"openai_mode": "standard",
	}
	if !IsEvaluatorOpenAIConfigured(cfg) {
		t.Fatalf("expected legacy api_key to satisfy OpenAI config")
	}

	provider := ResolveEvaluatorProvider(cfg)
	if provider != EvaluatorProviderOpenAI {
		t.Fatalf("expected %s, got %s", EvaluatorProviderOpenAI, provider)
	}
}

func TestResolveEvaluatorProvider_AutoWithLegacyAPIKeyPrefersOpenAI(t *testing.T) {
	cfg := map[string]any{
		"llm_provider": "auto",
		"api_key":      "sk-legacy-openai",
		"openai_mode":  "standard",
	}

	provider := ResolveEvaluatorProvider(cfg)
	if provider != EvaluatorProviderOpenAI {
		t.Fatalf("expected %s, got %s", EvaluatorProviderOpenAI, provider)
	}
}

func TestIsSelectedEvaluatorProviderConfigured_AutoWithLegacyAPIKeyRemainsValid(t *testing.T) {
	cfg := map[string]any{
		"llm_provider": "auto",
		"api_key":      "sk-legacy-openai",
		"openai_mode":  "standard",
	}

	if !IsSelectedEvaluatorProviderConfigured(cfg) {
		t.Fatalf("expected auto selection with legacy api_key to be valid through OpenAI fallback")
	}
}

func TestIsSelectedEvaluatorProviderConfigured_RespectsUserChoice(t *testing.T) {
	cfg := map[string]any{
		"llm_provider":   "openai",
		"nvidia_api_key": "nvapi-123",
	}
	if IsSelectedEvaluatorProviderConfigured(cfg) {
		t.Fatalf("expected openai selection without openai key to be invalid")
	}

	cfg["openai_api_key"] = "sk-openai"
	cfg["openai_mode"] = "standard"
	if !IsSelectedEvaluatorProviderConfigured(cfg) {
		t.Fatalf("expected openai selection with openai key to be valid")
	}
}

func TestIsSelectedEvaluatorProviderConfigured_AutoAcceptsAnyConfiguredProvider(t *testing.T) {
	cfg := map[string]any{
		"llm_provider":       "auto",
		"openrouter_api_key": "or-123",
	}
	if !IsSelectedEvaluatorProviderConfigured(cfg) {
		t.Fatalf("expected auto selection with openrouter key to be valid")
	}
}
