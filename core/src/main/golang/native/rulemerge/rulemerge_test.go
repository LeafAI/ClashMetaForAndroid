package rulemerge

import (
	"reflect"
	"strings"
	"testing"
)

const verge = `# Profile Enhancement Merge Template for Clash Verge

profile:
  store-selected: true

rules:
  - GEOSITE,demo,🤖 AI服务
  - DOMAIN-SUFFIX,x.ai,🤖 AI服务
  - DOMAIN-SUFFIX,github.com,🚀 节点选择
  - IP-CIDR,104.168.229.23/32,🚀 节点选择
`

func TestParseVergeTemplate(t *testing.T) {
	set, err := Parse(verge)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	want := []string{
		"GEOSITE,demo,🤖 AI服务",
		"DOMAIN-SUFFIX,x.ai,🤖 AI服务",
		"DOMAIN-SUFFIX,github.com,🚀 节点选择",
		"IP-CIDR,104.168.229.23/32,🚀 节点选择",
	}

	if !reflect.DeepEqual(set.Prepend, want) {
		t.Fatalf("Prepend = %v, want %v", set.Prepend, want)
	}

	if len(set.Append) != 0 {
		t.Fatalf("Append = %v, want empty", set.Append)
	}
}

func TestParsePrependAndAppend(t *testing.T) {
	set, err := Parse("prepend-rules:\n  - DOMAIN,a.com,DIRECT\nrules:\n  - DOMAIN,b.com,DIRECT\nappend-rules:\n  - MATCH,Proxy\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if want := []string{"DOMAIN,a.com,DIRECT", "DOMAIN,b.com,DIRECT"}; !reflect.DeepEqual(set.Prepend, want) {
		t.Fatalf("Prepend = %v, want %v", set.Prepend, want)
	}

	if want := []string{"MATCH,Proxy"}; !reflect.DeepEqual(set.Append, want) {
		t.Fatalf("Append = %v, want %v", set.Append, want)
	}
}

func TestParseEmptyAndComments(t *testing.T) {
	for _, src := range []string{"", "   \n", "# only a comment\n", "profile:\n  store-selected: true\n"} {
		set, err := Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}

		if len(set.Prepend) != 0 || len(set.Append) != 0 {
			t.Fatalf("Parse(%q) = %+v, want empty", src, set)
		}
	}
}

func TestParseTrimsAndDropsBlankRules(t *testing.T) {
	set, err := Parse("rules:\n  - \"  DOMAIN,a.com,DIRECT  \"\n  - \"\"\n  - '   '\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if want := []string{"DOMAIN,a.com,DIRECT"}; !reflect.DeepEqual(set.Prepend, want) {
		t.Fatalf("Prepend = %v, want %v", set.Prepend, want)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"invalid yaml":      "rules: [\n",
		"rules not a list":  "rules: DOMAIN,a.com,DIRECT\n",
		"rule not a string": "rules:\n  - {a: b}\n",
		"top-level list":    "- a\n- b\n",
		"missing target":    "rules:\n  - DOMAIN,a.com\n",
		"unknown type":      "rules:\n  - FOO,a.com,DIRECT\n",
		"missing payload":   "rules:\n  - DOMAIN\n",
	}

	for name, src := range cases {
		if _, err := Parse(src); err == nil {
			t.Errorf("%s: Parse(%q) succeeded, want error", name, src)
		}
	}
}

func TestParseErrorMentionsLine(t *testing.T) {
	_, err := Parse("rules:\n  - DOMAIN,a.com,DIRECT\n  - DOMAIN,b.com\n")
	if err == nil || !strings.Contains(err.Error(), "DOMAIN,b.com") {
		t.Fatalf("error should name the offending rule, got %v", err)
	}
}

func TestMerge(t *testing.T) {
	set := RuleSet{
		Prepend: []string{"DOMAIN,a.com,Proxy", "DOMAIN,b.com,Missing", "DOMAIN,c.com,DIRECT"},
		Append:  []string{"MATCH,Proxy", "DOMAIN,d.com,Gone"},
	}
	existing := []string{"DOMAIN,old.com,DIRECT", "MATCH,DIRECT"}
	targets := map[string]struct{}{"Proxy": {}, "DIRECT": {}}

	merged, skipped := set.Merge(existing, func(name string) bool {
		_, ok := targets[name]

		return ok
	})

	want := []string{
		"DOMAIN,a.com,Proxy",
		"DOMAIN,c.com,DIRECT",
		"DOMAIN,old.com,DIRECT",
		"MATCH,DIRECT",
		"MATCH,Proxy",
	}
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("merged = %v, want %v", merged, want)
	}

	if len(skipped) != 2 {
		t.Fatalf("skipped = %v, want 2 entries", skipped)
	}
}

func TestMergeDoesNotMutateInputs(t *testing.T) {
	set := RuleSet{Prepend: []string{"DOMAIN,a.com,DIRECT"}}
	existing := make([]string, 1, 8)
	existing[0] = "MATCH,DIRECT"

	merged, _ := set.Merge(existing, func(string) bool { return true })

	if len(existing) != 1 || existing[0] != "MATCH,DIRECT" {
		t.Fatalf("existing mutated: %v", existing)
	}

	merged[0] = "changed"

	if set.Prepend[0] != "DOMAIN,a.com,DIRECT" {
		t.Fatalf("set mutated through result: %v", set.Prepend)
	}
}

func TestMergeEmptySetKeepsExisting(t *testing.T) {
	existing := []string{"MATCH,DIRECT"}

	merged, skipped := RuleSet{}.Merge(existing, func(string) bool { return true })

	if !reflect.DeepEqual(merged, existing) || len(skipped) != 0 {
		t.Fatalf("merged=%v skipped=%v", merged, skipped)
	}
}

func TestMergeLogicalRulesCheckTrailingTarget(t *testing.T) {
	set := RuleSet{Prepend: []string{
		"AND,((DOMAIN,a.com),(NETWORK,UDP)),Proxy",
		"AND,((DOMAIN,a.com),(NETWORK,UDP)),Gone",
	}}

	merged, skipped := set.Merge(nil, func(name string) bool { return name == "Proxy" })

	if len(merged) != 1 || len(skipped) != 1 {
		t.Fatalf("merged=%v skipped=%v", merged, skipped)
	}
}
