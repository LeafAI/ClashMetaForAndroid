package config

import (
	"strings"

	"github.com/metacubex/mihomo/config"
	"github.com/metacubex/mihomo/log"

	"cfa/native/rulemerge"
)

var builtinTargets = map[string]struct{}{
	"DIRECT": {}, "REJECT": {}, "REJECT-DROP": {}, "PASS": {}, "COMPATIBLE": {}, "GLOBAL": {},
}

// patchRules merges the user's global rule source into the profile rules.
// Rules pointing at an outbound the profile does not define are skipped rather
// than failing the whole profile.
func patchRules(cfg *config.RawConfig, _ string) error {
	set, err := rulemerge.Parse(ReadRuleSource())
	if err != nil {
		log.Warnln("Apply rule source: %s", err.Error())

		return nil
	}

	if len(set.Prepend) == 0 && len(set.Append) == 0 {
		return nil
	}

	merged, skipped := set.Merge(cfg.Rule, targetChecker(cfg))
	for _, rule := range skipped {
		log.Warnln("Skip rule [%s]: target not found in profile", rule)
	}

	cfg.Rule = merged

	return nil
}

func targetChecker(cfg *config.RawConfig) func(string) bool {
	names := make(map[string]struct{}, len(builtinTargets)+len(cfg.Proxy)+len(cfg.ProxyGroup))

	for name := range builtinTargets {
		names[name] = struct{}{}
	}

	for _, proxy := range cfg.Proxy {
		if name, ok := proxy["name"].(string); ok {
			names[name] = struct{}{}
		}
	}

	for _, group := range cfg.ProxyGroup {
		if name, ok := group["name"].(string); ok {
			names[name] = struct{}{}
		}
	}

	subRules := cfg.SubRules

	return func(target string) bool {
		if _, ok := names[target]; ok {
			return true
		}

		_, ok := subRules[strings.TrimSpace(target)]

		return ok
	}
}

// ValidateRuleSource reports whether source is a well formed rule source.
func ValidateRuleSource(source string) error {
	_, err := rulemerge.Parse(source)

	return err
}
