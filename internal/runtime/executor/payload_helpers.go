package executor

import (
	"encoding/json"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// applyPayloadConfigWithRoot behaves like applyPayloadConfig but treats all parameter
// paths as relative to the provided root path (for example, "request" for Gemini CLI)
// and restricts matches to the given protocol when supplied. Defaults are checked
// against the original payload when provided. requestedModel carries the client-visible
// model name before alias resolution so payload rules can target aliases precisely.
func applyPayloadConfigWithRoot(cfg *config.Config, model, protocol, root string, payload, original []byte, requestedModel string) []byte {
	if cfg == nil || len(payload) == 0 {
		return payload
	}
	rules := cfg.Payload
	if len(rules.Default) == 0 && len(rules.DefaultRaw) == 0 && len(rules.Override) == 0 && len(rules.OverrideRaw) == 0 {
		return payload
	}
	model = strings.TrimSpace(model)
	requestedModel = strings.TrimSpace(requestedModel)
	if model == "" && requestedModel == "" {
		return payload
	}
	candidates := payloadModelCandidates(model, requestedModel)
	out := payload
	source := original
	if len(source) == 0 {
		source = payload
	}
	appliedDefaults := make(map[string]struct{})
	// Apply default rules: first write wins per field across all matching rules.
	for i := range rules.Default {
		rule := &rules.Default[i]
		if !payloadRuleMatchesModels(rule, protocol, candidates) {
			continue
		}
		for path, value := range rule.Params {
			fullPath := buildPayloadPath(root, path)
			if fullPath == "" {
				continue
			}
			if gjson.GetBytes(source, fullPath).Exists() {
				continue
			}
			if _, ok := appliedDefaults[fullPath]; ok {
				continue
			}
			updated, errSet := sjson.SetBytes(out, fullPath, value)
			if errSet != nil {
				continue
			}
			out = updated
			appliedDefaults[fullPath] = struct{}{}
		}
	}
	// Apply default raw rules: first write wins per field across all matching rules.
	for i := range rules.DefaultRaw {
		rule := &rules.DefaultRaw[i]
		if !payloadRuleMatchesModels(rule, protocol, candidates) {
			continue
		}
		for path, value := range rule.Params {
			fullPath := buildPayloadPath(root, path)
			if fullPath == "" {
				continue
			}
			if gjson.GetBytes(source, fullPath).Exists() {
				continue
			}
			if _, ok := appliedDefaults[fullPath]; ok {
				continue
			}
			rawValue, ok := payloadRawValue(value)
			if !ok {
				continue
			}
			updated, errSet := sjson.SetRawBytes(out, fullPath, rawValue)
			if errSet != nil {
				continue
			}
			out = updated
			appliedDefaults[fullPath] = struct{}{}
		}
	}
	// Apply override rules: last write wins per field across all matching rules.
	for i := range rules.Override {
		rule := &rules.Override[i]
		if !payloadRuleMatchesModels(rule, protocol, candidates) {
			continue
		}
		for path, value := range rule.Params {
			fullPath := buildPayloadPath(root, path)
			if fullPath == "" {
				continue
			}
			updated, errSet := sjson.SetBytes(out, fullPath, value)
			if errSet != nil {
				continue
			}
			out = updated
		}
	}
	// Apply override raw rules: last write wins per field across all matching rules.
	for i := range rules.OverrideRaw {
		rule := &rules.OverrideRaw[i]
		if !payloadRuleMatchesModels(rule, protocol, candidates) {
			continue
		}
		for path, value := range rule.Params {
			fullPath := buildPayloadPath(root, path)
			if fullPath == "" {
				continue
			}
			rawValue, ok := payloadRawValue(value)
			if !ok {
				continue
			}
			updated, errSet := sjson.SetRawBytes(out, fullPath, rawValue)
			if errSet != nil {
				continue
			}
			out = updated
		}
	}
	return out
}

func payloadRuleMatchesModels(rule *config.PayloadRule, protocol string, models []string) bool {
	if rule == nil || len(models) == 0 {
		return false
	}
	for _, model := range models {
		if payloadRuleMatchesModel(rule, model, protocol) {
			return true
		}
	}
	return false
}

func payloadRuleMatchesModel(rule *config.PayloadRule, model, protocol string) bool {
	if rule == nil {
		return false
	}
	if len(rule.Models) == 0 {
		return false
	}
	for _, entry := range rule.Models {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		if ep := strings.TrimSpace(entry.Protocol); ep != "" && protocol != "" && !strings.EqualFold(ep, protocol) {
			continue
		}
		if matchModelPattern(name, model) {
			return true
		}
	}
	return false
}

func payloadModelCandidates(model, requestedModel string) []string {
	model = strings.TrimSpace(model)
	requestedModel = strings.TrimSpace(requestedModel)
	if model == "" && requestedModel == "" {
		return nil
	}
	candidates := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)
	addCandidate := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, value)
	}
	if model != "" {
		addCandidate(model)
	}
	if requestedModel != "" {
		parsed := thinking.ParseSuffix(requestedModel)
		base := strings.TrimSpace(parsed.ModelName)
		if base != "" {
			addCandidate(base)
		}
		if parsed.HasSuffix {
			addCandidate(requestedModel)
		}
	}
	return candidates
}

// buildPayloadPath combines an optional root path with a relative parameter path.
// When root is empty, the parameter path is used as-is. When root is non-empty,
// the parameter path is treated as relative to root.
func buildPayloadPath(root, path string) string {
	r := strings.TrimSpace(root)
	p := strings.TrimSpace(path)
	if r == "" {
		return p
	}
	if p == "" {
		return r
	}
	if strings.HasPrefix(p, ".") {
		p = p[1:]
	}
	return r + "." + p
}

func payloadRawValue(value any) ([]byte, bool) {
	if value == nil {
		return nil, false
	}
	switch typed := value.(type) {
	case string:
		return []byte(typed), true
	case []byte:
		return typed, true
	default:
		raw, errMarshal := json.Marshal(typed)
		if errMarshal != nil {
			return nil, false
		}
		return raw, true
	}
}

func payloadRequestedModel(opts cliproxyexecutor.Options, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if len(opts.Metadata) == 0 {
		return fallback
	}
	raw, ok := opts.Metadata[cliproxyexecutor.RequestedModelMetadataKey]
	if !ok || raw == nil {
		return fallback
	}
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return fallback
		}
		return strings.TrimSpace(v)
	case []byte:
		if len(v) == 0 {
			return fallback
		}
		trimmed := strings.TrimSpace(string(v))
		if trimmed == "" {
			return fallback
		}
		return trimmed
	default:
		return fallback
	}
}

// matchModelPattern performs simple wildcard matching where '*' matches zero or more characters.
// Examples:
//
//	"*-5" matches "gpt-5"
//	"gpt-*" matches "gpt-5" and "gpt-4"
//	"gemini-*-pro" matches "gemini-2.5-pro" and "gemini-3-pro".
func matchModelPattern(pattern, model string) bool {
	pattern = strings.TrimSpace(pattern)
	model = strings.TrimSpace(model)
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	// Iterative glob-style matcher supporting only '*' wildcard.
	pi, si := 0, 0
	starIdx := -1
	matchIdx := 0
	for si < len(model) {
		if pi < len(pattern) && (pattern[pi] == model[si]) {
			pi++
			si++
			continue
		}
		if pi < len(pattern) && pattern[pi] == '*' {
			starIdx = pi
			matchIdx = si
			pi++
			continue
		}
		if starIdx != -1 {
			pi = starIdx + 1
			matchIdx++
			si = matchIdx
			continue
		}
		return false
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

// ApplyModelProcessor applies model-specific payload transformations.
func ApplyModelProcessor(payload []byte, processor string) []byte {
	switch strings.ToLower(strings.TrimSpace(processor)) {
	case "kimi-k2":
		return applyKimiK2Processor(payload)
	case "glm":
		return applyGLMProcessor(payload)
	default:
		return payload
	}
}

func applyKimiK2Processor(payload []byte) []byte {
	// 1. Read reasoning_effort and convert to thinking
	// Check root level first
	effort := gjson.GetBytes(payload, "reasoning_effort").String()
	effort = strings.ToLower(strings.TrimSpace(effort))

	// 2. Set thinking based on reasoning_effort
	// If reasoning_effort is explicitly "none" or "off", disable thinking
	if effort == "none" || effort == "off" {
		payload, _ = sjson.SetRawBytes(payload, "thinking", []byte(`{"type":"disabled"}`))
	} else {
		// Default to enabled for Kimi K2.5 (implied by model behavior, but explicit here)
		// We set it to enabled unless specifically disabled
		// Note: If thinking is already set, this overwrites it if reasoning_effort is present.
		// However, if reasoning_effort is NOT present, we might want to preserve existing thinking setting?
		// The prompt says: "none" or "off" is "thinking":{"type": "disabled"}, others "thinking": {"type": "enabled"}
		// So if reasoning_effort is missing (empty string), it falls into "others" -> enabled.
		// Let's check if reasoning_effort exists first.
		if gjson.GetBytes(payload, "reasoning_effort").Exists() {
			payload, _ = sjson.SetRawBytes(payload, "thinking", []byte(`{"type":"enabled"}`))
		}
	}

	// 3. Delete all unsupported OpenAI-specific fields and Kimi fixed-value fields
	// Kimi K2.5 has strict requirements on fixed fields (temperature, top_p, etc.)
	// and does not support many standard OpenAI fields.
	for _, field := range []string{
		"reasoning_effort",    // Not a Kimi field (used above, now delete)
		"temperature",         // K2.5: fixed value, rejected if set
		"top_p",               // K2.5: fixed value, rejected if set
		"n",                   // K2.5: fixed value, rejected if set
		"presence_penalty",    // K2.5: fixed value, rejected if set
		"frequency_penalty",   // K2.5: fixed value, rejected if set
		"logit_bias",          // Not supported
		"logprobs",            // Not supported
		"top_logprobs",        // Not supported
		"seed",                // Not supported
		"service_tier",        // Not supported
		"parallel_tool_calls", // Not supported
		"audio",               // Not supported
		"store",               // Not supported
		"modalities",          // Not supported
		"prediction",          // Not supported
		"stream_options",      // Not supported
		"user",                // Not supported
		"metadata",            // Not supported
		"web_search_options",  // Not supported (Kimi has its own tool)
	} {
		payload, _ = sjson.DeleteBytes(payload, field)
	}
	return payload
}

func applyGLMProcessor(payload []byte) []byte {
	// Delete OpenAI-specific parameters not supported by GLM
	for _, field := range []string{
		"reasoning_effort",    // OpenAI o-series
		"n",                   // OpenAI: number of completions
		"presence_penalty",    // OpenAI penalty params
		"frequency_penalty",
		"logit_bias",
		"logprobs",
		"top_logprobs",
		"seed",
		"service_tier",
		"parallel_tool_calls",
		"audio",               // OpenAI audio param (GLM uses different model)
		"store",
		"modalities",
		"prediction",
		"stream_options",      // GLM uses different stream handling
		"user",                // GLM uses "user_id" instead
		"metadata",
		"web_search_options",
	} {
		payload, _ = sjson.DeleteBytes(payload, field)
	}
	return payload
}
