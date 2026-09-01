package v1alpha1

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestPluginStatusRoundTrip(t *testing.T) {
	in := &Plugin{}
	in.Status.ObservedGeneration = 5
	in.Status.SetCondition(Condition{Type: "Ready", Status: ConditionTrue, Reason: "Resolved"})
	in.Status.ResolvedSource = &PluginResolvedSource{Type: PluginSourceTypeGit, Commit: "abc123"}
	in.Status.Formats = []string{PluginFormatAgentPlugins, PluginFormatClaudePlugin}
	in.Status.Manifests = map[string]*PluginManifest{
		PluginFormatClaudePlugin: {Name: "deploy", Version: "1.2.0"},
		PluginFormatAgentPlugins: {Name: "deploy", Version: "2.0.0"},
	}
	in.Status.MCPServerFiles = []string{".mcp.json", "mcp.json"}
	in.Status.Inventory = &PluginInventory{Skills: []PluginSkill{{Name: "deploy", Description: "Deploys"}}}

	raw, err := in.MarshalStatus()
	if err != nil {
		t.Fatalf("MarshalStatus: %v", err)
	}

	out := &Plugin{}
	if err := out.UnmarshalStatus(raw); err != nil {
		t.Fatalf("UnmarshalStatus: %v", err)
	}

	if out.Status.ObservedGeneration != 5 {
		t.Errorf("observedGeneration = %d, want 5", out.Status.ObservedGeneration)
	}
	if !out.Status.IsConditionTrue("Ready") {
		t.Error("Ready condition did not round-trip")
	}
	if out.Status.ResolvedSource == nil || out.Status.ResolvedSource.Commit != "abc123" || out.Status.ResolvedSource.Type != PluginSourceTypeGit {
		t.Errorf("resolvedSource did not round-trip: %+v", out.Status.ResolvedSource)
	}
	if out.Status.Inventory == nil || len(out.Status.Inventory.Skills) != 1 || out.Status.Inventory.Skills[0].Name != "deploy" {
		t.Errorf("inventory did not round-trip: %+v", out.Status.Inventory)
	}
	// The Plugin status codec is hand-written with an explicit key list, so a
	// new field that is not added to BOTH halves silently never persists.
	if !slices.Equal(out.Status.Formats, []string{PluginFormatAgentPlugins, PluginFormatClaudePlugin}) {
		t.Errorf("formats did not round-trip: %v", out.Status.Formats)
	}
	if len(out.Status.Manifests) != 2 {
		t.Fatalf("manifests did not round-trip: %+v", out.Status.Manifests)
	}
	if got := out.Status.Manifests[PluginFormatAgentPlugins].Version; got != "2.0.0" {
		t.Errorf("agent-plugins manifest version = %q, want 2.0.0", got)
	}
	if !slices.Equal(out.Status.MCPServerFiles, []string{".mcp.json", "mcp.json"}) {
		t.Errorf("mcpServerFiles did not round-trip: %v", out.Status.MCPServerFiles)
	}
}

// TestPluginStatusAdoptsLegacyManifestKey covers the one-way migration: a
// status persisted before status.manifest was removed carries that key and no
// manifests, and the writer never emits it again. Without the adoption the
// marketplace silently loses the plugin's description and version until the
// controller happens to re-reconcile it.
func TestPluginStatusAdoptsLegacyManifestKey(t *testing.T) {
	stored := `{"manifest":{"name":"deploy","version":"1.2.0"}}`

	p := &Plugin{}
	if err := p.UnmarshalStatus([]byte(stored)); err != nil {
		t.Fatalf("UnmarshalStatus: %v", err)
	}
	got := p.Status.PreferredManifest()
	if got == nil || got.Version != "1.2.0" {
		t.Fatalf("legacy manifest not adopted: %+v", p.Status.Manifests)
	}
	if _, ok := p.Status.Manifests[PluginFormatClaudePlugin]; !ok {
		t.Errorf("legacy manifest must key as %q, got %+v", PluginFormatClaudePlugin, p.Status.Manifests)
	}

	// A fresh status wins: adoption must never overwrite a real scan result.
	both := `{"manifest":{"name":"old","version":"0.1.0"},"manifests":{"claude-plugin":{"name":"new","version":"2.0.0"}}}`
	q := &Plugin{}
	if err := q.UnmarshalStatus([]byte(both)); err != nil {
		t.Fatalf("UnmarshalStatus: %v", err)
	}
	if got := q.Status.PreferredManifest(); got == nil || got.Version != "2.0.0" {
		t.Fatalf("manifests must win over the legacy key, got %+v", got)
	}
}

// TestPluginStatusOmitsNilCustomFields guards the patch-skip byte-stability
// contract: absent server-determined fields must not emit stray JSON keys.
func TestPluginStatusOmitsNilCustomFields(t *testing.T) {
	p := &Plugin{}
	p.Status.SetCondition(Condition{Type: "Ready", Status: ConditionFalse, Reason: "Progressing"})

	raw, err := p.MarshalStatus()
	if err != nil {
		t.Fatalf("MarshalStatus: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"resolvedSource", "manifest", "manifests", "formats", "mcpServerFiles", "inventory"} {
		if _, ok := m[k]; ok {
			t.Errorf("nil %q must be omitted, got key in %s", k, string(raw))
		}
	}
}
