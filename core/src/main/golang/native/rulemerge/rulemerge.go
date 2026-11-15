// Package rulemerge parses a user supplied rule source (a small YAML document
// in the style of a Clash Verge merge template) and merges its rules into a
// profile's rule list.
//
// The package is deliberately free of any mihomo dependency so that it can be
// unit tested on the host.
package rulemerge

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// RuleSet is the parsed content of a rule source.
type RuleSet struct {
	// Prepend rules are placed before the profile's own rules.
	Prepend []string
	// Append rules are placed after the profile's own rules.
	Append []string
}

type document struct {
	PrependRules []string `yaml:"prepend-rules"`
	Rules        []string `yaml:"rules"`
	AppendRules  []string `yaml:"append-rules"`
}

// ruleTypes lists the rule types mihomo understands.
var ruleTypes = map[string]struct{}{
	"DOMAIN": {}, "DOMAIN-SUFFIX": {}, "DOMAIN-KEYWORD": {}, "DOMAIN-REGEX": {}, "DOMAIN-WILDCARD": {},
	"GEOSITE": {}, "GEOIP": {}, "SRC-GEOIP": {}, "IP-ASN": {}, "SRC-IP-ASN": {},
	"IP-CIDR": {}, "IP-CIDR6": {}, "SRC-IP-CIDR": {}, "IP-SUFFIX": {}, "SRC-IP-SUFFIX": {},
	"SRC-PORT": {}, "DST-PORT": {}, "IN-PORT": {}, "DSCP": {},
	"PROCESS-NAME": {}, "PROCESS-PATH": {}, "PROCESS-NAME-REGEX": {}, "PROCESS-PATH-REGEX": {},
	"PROCESS-NAME-WILDCARD": {}, "PROCESS-PATH-WILDCARD": {},
	"NETWORK": {}, "UID": {}, "IN-TYPE": {}, "IN-USER": {}, "IN-NAME": {}, "REMATCH-NAME": {},
	"SUB-RULE": {}, "AND": {}, "OR": {}, "NOT": {}, "RULE-SET": {}, "MATCH": {},
}

// Parse reads a rule source. An empty source, or one without any rule keys,
// yields an empty RuleSet. Keys other than the rule lists (for example
// "profile") are accepted and ignored so that a Clash Verge template can be
// pasted unchanged.
//
// "prepend-rules" and "rules" are both placed in front of the profile's own
// rules, in that order. "append-rules" goes after them.
func Parse(source string) (RuleSet, error) {
	if strings.TrimSpace(source) == "" {
		return RuleSet{}, nil
	}

	var doc document
	if err := yaml.Unmarshal([]byte(source), &doc); err != nil {
		return RuleSet{}, fmt.Errorf("invalid rule source: %w", err)
	}

	prepend := cleanRules(append(append([]string{}, doc.PrependRules...), doc.Rules...))
	appended := cleanRules(doc.AppendRules)

	for _, rule := range append(append([]string{}, prepend...), appended...) {
		if err := validate(rule); err != nil {
			return RuleSet{}, err
		}
	}

	return RuleSet{Prepend: prepend, Append: appended}, nil
}

func cleanRules(rules []string) []string {
	cleaned := make([]string, 0, len(rules))

	for _, rule := range rules {
		if rule = strings.TrimSpace(rule); rule != "" {
			cleaned = append(cleaned, rule)
		}
	}

	return cleaned
}

func validate(rule string) error {
	parts := strings.Split(rule, ",")
	kind := strings.ToUpper(strings.TrimSpace(parts[0]))

	if _, ok := ruleTypes[kind]; !ok {
		return fmt.Errorf("rule [%s]: unknown rule type %q", rule, kind)
	}

	if kind == "MATCH" {
		if len(parts) < 2 || Target(rule) == "" {
			return fmt.Errorf("rule [%s]: missing target", rule)
		}

		return nil
	}

	if len(parts) < 3 || strings.TrimSpace(parts[1]) == "" || Target(rule) == "" {
		return fmt.Errorf("rule [%s]: expected TYPE,PAYLOAD,TARGET", rule)
	}

	return nil
}

// Target returns the outbound a rule points at. For logical rules (AND, OR,
// NOT, SUB-RULE) and DOMAIN-REGEX the payload may contain commas, so the target
// is always the last field; for the other rules it is the third field.
func Target(rule string) string {
	parts := strings.Split(rule, ",")
	kind := strings.ToUpper(strings.TrimSpace(parts[0]))

	switch kind {
	case "MATCH":
		if len(parts) > 1 {
			return strings.TrimSpace(parts[1])
		}
	case "AND", "OR", "NOT", "SUB-RULE", "DOMAIN-REGEX", "PROCESS-NAME-REGEX", "PROCESS-PATH-REGEX":
		if len(parts) > 2 {
			return strings.TrimSpace(parts[len(parts)-1])
		}
	default:
		if len(parts) > 2 {
			return strings.TrimSpace(parts[2])
		}
	}

	return ""
}

// Merge returns existing with the set's rules added around it. Rules whose
// target is rejected by hasTarget are left out and reported in skipped, so a
// rule source written for one profile cannot make another profile fail to load.
// Neither the receiver nor existing is modified.
func (s RuleSet) Merge(existing []string, hasTarget func(name string) bool) (merged, skipped []string) {
	merged = make([]string, 0, len(s.Prepend)+len(existing)+len(s.Append))

	keep := func(rules []string) {
		for _, rule := range rules {
			if hasTarget(Target(rule)) {
				merged = append(merged, rule)
			} else {
				skipped = append(skipped, rule)
			}
		}
	}

	keep(s.Prepend)
	merged = append(merged, existing...)
	keep(s.Append)

	return merged, skipped
}
