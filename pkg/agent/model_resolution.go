package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

func ensureProtocolModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if strings.Contains(model, "/") {
		return model
	}
	return "openai/" + model
}

func modelConfigIdentityKey(mc *config.ModelConfig) string {
	if mc == nil {
		return ""
	}
	if name := strings.TrimSpace(mc.ModelName); name != "" {
		return "model_name:" + name
	}
	return ""
}

func modelConfigResolutionKey(defaultProvider string, mc *config.ModelConfig) string {
	if mc == nil {
		return ""
	}
	provider, modelID := modelProviderAndIDForResolution(defaultProvider, mc)
	apiKeys := make([]string, len(mc.APIKeys))
	for i := range mc.APIKeys {
		apiKeys[i] = mc.APIKeys[i].String()
	}
	payload, err := json.Marshal(struct {
		ModelName           string                      `json:"model_name"`
		Provider            string                      `json:"provider"`
		Model               string                      `json:"model"`
		APIBase             string                      `json:"api_base"`
		Proxy               string                      `json:"proxy"`
		AuthMethod          string                      `json:"auth_method"`
		ConnectMode         string                      `json:"connect_mode"`
		Workspace           string                      `json:"workspace"`
		APIKeys             []string                    `json:"api_keys"`
		Enabled             bool                        `json:"enabled"`
		UserAgent           string                      `json:"user_agent"`
		RPM                 int                         `json:"rpm"`
		RequestTimeout      int                         `json:"request_timeout"`
		MaxTokensField      string                      `json:"max_tokens_field"`
		ThinkingLevel       string                      `json:"thinking_level"`
		ToolSchemaTransform string                      `json:"tool_schema_transform"`
		Streaming           config.ModelStreamingConfig `json:"streaming"`
		ExtraBody           map[string]any              `json:"extra_body"`
		CustomHeaders       map[string]string           `json:"custom_headers"`
	}{
		ModelName: mc.ModelName, Provider: provider, Model: modelID,
		APIBase: mc.APIBase, Proxy: mc.Proxy, AuthMethod: mc.AuthMethod,
		ConnectMode: mc.ConnectMode, Workspace: mc.Workspace, APIKeys: apiKeys,
		Enabled: mc.Enabled, UserAgent: mc.UserAgent, RPM: mc.RPM,
		RequestTimeout: mc.RequestTimeout, MaxTokensField: mc.MaxTokensField,
		ThinkingLevel: mc.ThinkingLevel, ToolSchemaTransform: mc.ToolSchemaTransform,
		Streaming: mc.Streaming, ExtraBody: mc.ExtraBody, CustomHeaders: mc.CustomHeaders,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("config:%x", sum)
}

func effectiveDefaultProvider(defaultProvider string) string {
	defaultProvider = strings.TrimSpace(defaultProvider)
	if defaultProvider == "" {
		return "openai"
	}
	return providers.NormalizeProvider(defaultProvider)
}

func modelProviderAndIDForResolution(defaultProvider string, mc *config.ModelConfig) (provider string, modelID string) {
	if mc == nil {
		return "", ""
	}
	if strings.TrimSpace(mc.Provider) != "" {
		return providers.ExtractProtocol(mc)
	}
	return providers.SplitModelProviderAndID(mc.Model, effectiveDefaultProvider(defaultProvider))
}

func cloneModelConfigForResolution(
	defaultProvider string,
	mc *config.ModelConfig,
	workspace string,
) *config.ModelConfig {
	if mc == nil {
		return nil
	}
	clone := *mc
	if clone.Workspace == "" {
		clone.Workspace = workspace
	}
	return &clone
}

// hasVirtualKeySiblings reports whether mc is part of a multi-key expansion,
// i.e. there exist virtual "__key_N" entries derived from the same model name.
func hasVirtualKeySiblings(cfg *config.Config, mc *config.ModelConfig) bool {
	if cfg == nil || mc == nil {
		return false
	}
	base := mc.ModelName
	if i := strings.Index(base, "__key_"); i > 0 {
		base = base[:i]
	}
	for _, other := range cfg.ModelList {
		if other == nil || other == mc || !other.IsVirtual() {
			continue
		}
		if strings.HasPrefix(other.ModelName, base+"__key_") {
			return true
		}
	}
	return false
}

func candidateFromModelConfig(
	defaultProvider string,
	mc *config.ModelConfig,
	hasKeySiblings bool,
) (providers.FallbackCandidate, bool) {
	if mc == nil {
		return providers.FallbackCandidate{}, false
	}

	protocol, modelID := modelProviderAndIDForResolution(defaultProvider, mc)
	if strings.TrimSpace(modelID) == "" {
		return providers.FallbackCandidate{}, false
	}

	// Round-robin opt-in. Two distinct intents:
	//   - key_rotation: rotate across the virtual __key_N entries produced by
	//     multi-key expansion (same model/provider, different key).
	//   - provider_rotation: rotate across genuinely different provider/model
	//     entries listed as fallbacks.
	// A virtual entry only ever represents an extra key, so provider_rotation
	// alone must not cause key rotation, and vice versa.
	// Key rotation applies to the whole key family: the primary entry rotates
	// too when it has virtual __key_N siblings. Provider rotation only applies
	// to real (non-virtual) entries and only when there is no key family to
	// rotate — otherwise a provider_rotation flag would silently rotate keys.
	rotate := mc.KeyRotation && (mc.IsVirtual() || hasKeySiblings)
	if !rotate && !mc.IsVirtual() && mc.ProviderRotation && !hasKeySiblings {
		rotate = true
	}

	return providers.FallbackCandidate{
		Provider:    protocol,
		Model:       modelID,
		DisplayName: strings.TrimSpace(mc.ModelName),
		RPM:         mc.RPM,
		IdentityKey: modelConfigIdentityKey(mc),
		ConfigKey:   modelConfigResolutionKey(defaultProvider, mc),
		Rotate:      rotate,
	}, true
}

func lookupModelConfigByRef(cfg *config.Config, raw string, defaultProvider ...string) *config.ModelConfig {
	raw = strings.TrimSpace(raw)
	if raw == "" || cfg == nil {
		return nil
	}

	if mc, err := cfg.GetModelConfig(raw); err == nil && mc != nil && strings.TrimSpace(mc.Model) != "" {
		return mc
	}

	resolved, err := providers.ResolveModelConfig(cfg, raw)
	if err != nil {
		return nil
	}
	for _, modelCfg := range cfg.ModelList {
		if modelConfigsResolveToSameEntry(
			modelCfg,
			resolved,
			cfg.Agents.Defaults.Provider,
		) {
			return resolved
		}
	}
	return nil
}

func modelConfigsResolveToSameEntry(
	left,
	right *config.ModelConfig,
	defaultProvider string,
) bool {
	if left == nil || right == nil || left.IsVirtual() {
		return false
	}
	leftProvider, leftModel := modelProviderAndIDForResolution(defaultProvider, left)
	rightProvider, rightModel := providers.ExtractProtocol(right)
	return left.ModelName == right.ModelName &&
		providers.ModelKey(leftProvider, leftModel) == providers.ModelKey(rightProvider, rightModel) &&
		left.APIBase == right.APIBase &&
		left.APIKey() == right.APIKey() &&
		left.Enabled == right.Enabled &&
		left.AuthMethod == right.AuthMethod &&
		left.ConnectMode == right.ConnectMode
}

func resolveModelCandidate(
	cfg *config.Config,
	defaultProvider string,
	raw string,
) (providers.FallbackCandidate, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return providers.FallbackCandidate{}, false
	}
	defaultProvider = effectiveDefaultProvider(defaultProvider)

	if mc := lookupModelConfigByRef(cfg, raw, defaultProvider); mc != nil {
		candidate, ok := candidateFromModelConfig(defaultProvider, mc, hasVirtualKeySiblings(cfg, mc))
		if !ok {
			return providers.FallbackCandidate{}, false
		}
		for index, modelCfg := range cfg.ModelList {
			if modelCfg == mc || modelConfigsResolveToSameEntry(modelCfg, mc, defaultProvider) {
				candidate.ConfigIndex = index + 1
				candidate.ConfigKey = modelConfigResolutionKey(defaultProvider, modelCfg)
				break
			}
		}
		return candidate, true
	}

	ref := providers.ParseModelRef(raw, defaultProvider)
	if ref == nil {
		return providers.FallbackCandidate{}, false
	}

	return providers.FallbackCandidate{
		Provider:    ref.Provider,
		Model:       ref.Model,
		DisplayName: raw,
	}, true
}

func resolveModelCandidates(
	cfg *config.Config,
	defaultProvider string,
	primary string,
	fallbacks []string,
) []providers.FallbackCandidate {
	seen := make(map[string]bool)
	candidates := make([]providers.FallbackCandidate, 0, 1+len(fallbacks))

	addCandidate := func(raw string) {
		candidate, ok := resolveModelCandidate(cfg, defaultProvider, raw)
		if !ok {
			return
		}

		key := candidate.StableKey()
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, candidate)
	}

	addCandidate(primary)
	for _, fallback := range fallbacks {
		addCandidate(fallback)
	}

	return candidates
}

func resolvedCandidateModel(candidates []providers.FallbackCandidate, fallback string) string {
	if len(candidates) > 0 && strings.TrimSpace(candidates[0].Model) != "" {
		return candidates[0].Model
	}
	return fallback
}

func resolvedCandidateProvider(candidates []providers.FallbackCandidate, fallback string) string {
	if len(candidates) > 0 && strings.TrimSpace(candidates[0].Provider) != "" {
		return candidates[0].Provider
	}
	return fallback
}

func resolvedCandidateModelName(candidates []providers.FallbackCandidate, fallback string) string {
	if len(candidates) > 0 {
		if name := modelAliasFromCandidateIdentityKey(candidates[0].IdentityKey); strings.TrimSpace(name) != "" {
			return name
		}
		if displayName := strings.TrimSpace(candidates[0].DisplayName); displayName != "" {
			return displayName
		}
	}
	return strings.TrimSpace(fallback)
}

func resolvedRuntimeModelConfig(cfg *config.Config, modelName, workspace string) (*config.ModelConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	modelCfg, err := providers.ResolveModelConfig(cfg, strings.TrimSpace(modelName))
	if err != nil {
		return nil, err
	}
	if modelCfg.Workspace == "" {
		modelCfg.Workspace = workspace
	}
	return modelCfg, nil
}

func resolveActiveModelConfig(
	cfg *config.Config,
	workspace string,
	candidates []providers.FallbackCandidate,
	activeModel string,
	defaultProvider string,
) *config.ModelConfig {
	if cfg == nil {
		return nil
	}
	defaultProvider = effectiveDefaultProvider(defaultProvider)

	if len(candidates) > 0 {
		candidate := candidates[0]
		if modelCfg, err := resolvedCandidateModelConfig(cfg, candidate, workspace); err == nil {
			return modelCfg
		}
		identityKey := strings.TrimSpace(candidate.IdentityKey)
		if identityKey != "" {
			for _, mc := range cfg.ModelList {
				if mc == nil || modelConfigIdentityKey(mc) != identityKey {
					continue
				}
				protocol, modelID := modelProviderAndIDForResolution(defaultProvider, mc)
				if providers.ModelKey(protocol, modelID) == providers.ModelKey(candidate.Provider, candidate.Model) {
					return cloneModelConfigForResolution(defaultProvider, mc, workspace)
				}
			}
		}
		for _, mc := range cfg.ModelList {
			if mc == nil {
				continue
			}
			protocol, modelID := modelProviderAndIDForResolution(defaultProvider, mc)
			if providers.ModelKey(protocol, modelID) == providers.ModelKey(candidate.Provider, candidate.Model) {
				return cloneModelConfigForResolution(defaultProvider, mc, workspace)
			}
		}
		return nil
	}

	if mc := lookupModelConfigByRef(cfg, activeModel, defaultProvider); mc != nil {
		return cloneModelConfigForResolution(defaultProvider, mc, workspace)
	}

	return nil
}
