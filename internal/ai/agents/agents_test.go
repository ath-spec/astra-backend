package agents

import "testing"

func TestDefaults_EveryAgentHasAModelList(t *testing.T) {
	c := New(Overrides{})
	for _, k := range []Key{
		KeyAppChat, KeyAppQuickChat, KeyRMCopilot, KeyAdminCopilot, KeyRMNarrator, KeyMemory,
	} {
		a := c.Get(k)
		if a.Key != k {
			t.Errorf("Get(%q).Key = %q", k, a.Key)
		}
		if len(a.Models) == 0 {
			t.Errorf("agent %q has no model fallback list", k)
		}
	}
}

func TestDefaults_PreserveInlineBehaviour(t *testing.T) {
	c := New(Overrides{})

	chat := c.Get(KeyAppChat)
	if len(chat.Models) != 2 || chat.Models[0] != "openai/gpt-oss-120b" || chat.Models[1] != "openai/gpt-oss-20b" {
		t.Errorf("app_chat model list drifted from inline behaviour: %v", chat.Models)
	}
	if chat.Temperature != nil {
		t.Errorf("app_chat should leave temperature unset, got %v", *chat.Temperature)
	}

	nar := c.Get(KeyRMNarrator)
	if nar.ResponseFormat != "json_object" {
		t.Errorf("rm_narrator must request json_object, got %q", nar.ResponseFormat)
	}
	if nar.MaxTokens != 2048 {
		t.Errorf("rm_narrator max tokens = %d, want 2048", nar.MaxTokens)
	}
	if nar.Temperature == nil || *nar.Temperature != 0.2 {
		t.Errorf("rm_narrator temperature = %v, want 0.2", nar.Temperature)
	}

	mem := c.Get(KeyMemory)
	if mem.Temperature == nil || *mem.Temperature != 0 {
		t.Errorf("memory extractor must pin temperature 0, got %v", mem.Temperature)
	}
}

func TestOverrides_BedrockIDAndModels(t *testing.T) {
	c := New(Overrides{
		BedrockAgentIDs: map[Key]string{KeyRMCopilot: "  bdrk-rm-9  "},
		Models:          map[Key][]string{KeyAppQuickChat: {"fast-1"}},
	})
	if got := c.Get(KeyRMCopilot).BedrockAgentID; got != "bdrk-rm-9" {
		t.Errorf("bedrock id override (trimmed) = %q", got)
	}
	if got := c.Get(KeyAppQuickChat).Models; len(got) != 1 || got[0] != "fast-1" {
		t.Errorf("model override = %v", got)
	}
	// Untouched agents keep their defaults.
	if got := c.Get(KeyAppChat).BedrockAgentID; got != "" {
		t.Errorf("app_chat should have no bedrock id, got %q", got)
	}
}

func TestGet_UnknownKeyFallsBackToAppChat(t *testing.T) {
	c := New(Overrides{})
	if got := c.Get(Key("does-not-exist")); got.Key != KeyAppChat {
		t.Errorf("unknown key fell back to %q, want app_chat", got.Key)
	}
}

func TestRequest_CarriesRoutingAndIsCopySafe(t *testing.T) {
	c := New(Overrides{BedrockAgentIDs: map[Key]string{KeyAppChat: "bdrk-1"}})
	a := c.Get(KeyAppChat)
	r := a.Request("sys", nil)
	if r.System != "sys" || r.Agent != "bdrk-1" || len(r.Models) != len(a.Models) {
		t.Fatalf("request scaffold wrong: %+v", r)
	}
	// Mutating the request's model slice must not corrupt the catalog.
	r.Models[0] = "mutated"
	if c.Get(KeyAppChat).Models[0] == "mutated" {
		t.Error("Request() leaked the catalog's backing array")
	}
}
