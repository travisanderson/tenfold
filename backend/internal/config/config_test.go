package config

import (
	"strings"
	"testing"
)

func TestAIRolesAreCheckedAgainstTheirKeys(t *testing.T) {
	cases := map[string]struct {
		c    Config
		want string // part of the one expected problem; empty means none
	}{
		"the defaults need no key": {
			Config{JudgeProvider: "mock", ShadowJudgeProvider: "none", NarrativeProvider: "mock"}, ""},
		"jev judges, claude shadows and narrates": {
			Config{JudgeProvider: "jev", ShadowJudgeProvider: "claude", NarrativeProvider: "claude", JevAPIKey: "k", AnthropicAPIKey: "k"}, ""},
		"jev alone needs only its own key": {
			Config{JudgeProvider: "jev", ShadowJudgeProvider: "none", NarrativeProvider: "mock", JevAPIKey: "k"}, ""},
		"jev without its key": {
			Config{JudgeProvider: "jev", ShadowJudgeProvider: "none", NarrativeProvider: "mock"}, "JEV_API_KEY is required when AI_JUDGE_PROVIDER is jev"},
		"a claude shadow without its key": {
			Config{JudgeProvider: "mock", ShadowJudgeProvider: "claude", NarrativeProvider: "mock"}, "ANTHROPIC_API_KEY is required when AI_SHADOW_JUDGE_PROVIDER is claude"},
		"jev cannot narrate": {
			Config{JudgeProvider: "mock", ShadowJudgeProvider: "none", NarrativeProvider: "jev", JevAPIKey: "k"}, `AI_NARRATIVE_PROVIDER "jev" is not one of`},
		"a judge shadowing itself": {
			Config{JudgeProvider: "mock", ShadowJudgeProvider: "mock", NarrativeProvider: "mock"}, "must differ"},
		"a misspelled provider": {
			Config{JudgeProvider: "cluade", ShadowJudgeProvider: "none", NarrativeProvider: "mock"}, `AI_JUDGE_PROVIDER "cluade" is not one of`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("AI_PROVIDER", "")
			got := tc.c.aiProblems()
			if tc.want == "" && len(got) > 0 {
				t.Errorf("problems = %v, want none", got)
			}
			if tc.want != "" && (len(got) != 1 || !strings.Contains(got[0], tc.want)) {
				t.Errorf("problems = %v, want one containing %q", got, tc.want)
			}
		})
	}
}

func TestTheOldProviderSettingIsRefusedNotIgnored(t *testing.T) {
	t.Setenv("AI_PROVIDER", "jev")
	got := Config{JudgeProvider: "mock", ShadowJudgeProvider: "none", NarrativeProvider: "mock"}.aiProblems()
	if len(got) != 1 || !strings.Contains(got[0], "AI_PROVIDER is no longer read") {
		t.Errorf("problems = %v", got)
	}
}
