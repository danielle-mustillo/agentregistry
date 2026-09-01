package bundle

import (
	"slices"
	"testing"

	"github.com/agentregistry-dev/agentregistry/pkg/api/v1alpha1"
)

const (
	claudeManifest       = `{"name":"demo","version":"1.0.0"}`
	agentPluginsManifest = `{"$schema":"https://agent-plugins.org/schemas/plugin/1.0.0/plugin.schema.json","name":"demo","version":"1.0.0"}`
	// foreignRootManifest is the shape that must NOT be read as agent-plugins:
	// a root plugin.json belonging to some unrelated ecosystem.
	foreignRootManifest = `{"name":"eslint-plugin-thing","main":"index.js"}`
	skillFile           = "---\nname: demo\ndescription: a demo\n---\nbody\n"
)

func TestDetectFormats(t *testing.T) {
	tests := []struct {
		name  string
		files map[string][]byte
		want  []string
	}{
		{
			name:  "claude manifest only",
			files: map[string][]byte{ClaudeManifestPath: []byte(claudeManifest)},
			want:  []string{v1alpha1.PluginFormatClaudePlugin},
		},
		{
			name:  "agent-plugins manifest only",
			files: map[string][]byte{AgentPluginsManifestPath: []byte(agentPluginsManifest)},
			want:  []string{v1alpha1.PluginFormatAgentPlugins},
		},
		{
			name: "dual format",
			files: map[string][]byte{
				ClaudeManifestPath:       []byte(claudeManifest),
				AgentPluginsManifestPath: []byte(agentPluginsManifest),
			},
			want: []string{v1alpha1.PluginFormatAgentPlugins, v1alpha1.PluginFormatClaudePlugin},
		},
		{
			name:  "manifest-less bundle with skills",
			files: map[string][]byte{"skills/demo/SKILL.md": []byte(skillFile)},
			want:  []string{v1alpha1.PluginFormatClaudePlugin},
		},
		{
			name:  "manifest-less bundle with commands",
			files: map[string][]byte{"commands/deploy.md": []byte("# deploy")},
			want:  []string{v1alpha1.PluginFormatClaudePlugin},
		},
		{
			name:  "manifest-less bundle with .mcp.json",
			files: map[string][]byte{ClaudeMCPPath: []byte(`{"mcpServers":{"a":{}}}`)},
			want:  []string{v1alpha1.PluginFormatClaudePlugin},
		},
		{
			// The whole point of the $schema guard: a foreign root plugin.json
			// is not an agent-plugins manifest, and on its own is not a plugin
			// bundle at all.
			name:  "foreign root plugin.json detects nothing",
			files: map[string][]byte{AgentPluginsManifestPath: []byte(foreignRootManifest)},
			want:  nil,
		},
		{
			// An agent-plugins bundle also ships skills/. The manifest-less
			// fallback must NOT fire here, or it would detect claude-plugin,
			// pass the harness gate on that strength, and drop its mcp.json
			// servers — the half-materialization the gate exists to prevent.
			name: "agent-plugins bundle with skills is not also claude-plugin",
			files: map[string][]byte{
				AgentPluginsManifestPath: []byte(agentPluginsManifest),
				"skills/demo/SKILL.md":   []byte(skillFile),
				AgentPluginsMCPPath:      []byte(`{"mcpServers":{"a":{}}}`),
			},
			want: []string{v1alpha1.PluginFormatAgentPlugins},
		},
		{
			name:  "nothing recognizable",
			files: map[string][]byte{"README.md": []byte("hi")},
			want:  nil,
		},
		{
			name:  "empty bundle",
			files: map[string][]byte{},
			want:  nil,
		},
		{
			// A malformed root plugin.json must not be treated as
			// agent-plugins, and must not panic the probe.
			name:  "malformed root plugin.json detects nothing",
			files: map[string][]byte{AgentPluginsManifestPath: []byte("{not json")},
			want:  nil,
		},
		{
			// Prefix match, not exact URL: a spec version bump must not
			// silently stop detecting the format.
			name: "future agent-plugins schema version still detected",
			files: map[string][]byte{
				AgentPluginsManifestPath: []byte(`{"$schema":"https://agent-plugins.org/schemas/plugin/9.9.9/plugin.schema.json","name":"demo"}`),
			},
			want: []string{v1alpha1.PluginFormatAgentPlugins},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectFormats(&CanonicalBundle{Files: tt.files})
			if !slices.Equal(got, tt.want) {
				t.Errorf("DetectFormats() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseManifestsDualFormat(t *testing.T) {
	b := &CanonicalBundle{Files: map[string][]byte{
		ClaudeManifestPath:       []byte(`{"name":"claude-side","version":"1.0.0"}`),
		AgentPluginsManifestPath: []byte(agentPluginsManifest),
	}}
	manifests, err := ParseManifests(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(manifests) != 2 {
		t.Fatalf("got %d manifests, want 2: %+v", len(manifests), manifests)
	}
	if got := manifests[v1alpha1.PluginFormatClaudePlugin].Name; got != "claude-side" {
		t.Errorf("claude manifest name = %q, want %q", got, "claude-side")
	}
	if got := manifests[v1alpha1.PluginFormatAgentPlugins].Schema; got == "" {
		t.Error("agent-plugins manifest lost its $schema")
	}
	// Single-manifest consumers get the Claude side, because that is what
	// every current harness actually reads.
	preferred := v1alpha1.PluginStatus{Manifests: manifests}.PreferredManifest()
	if preferred == nil || preferred.Name != "claude-side" {
		t.Errorf("PreferredManifest() = %+v, want the claude-side manifest", preferred)
	}
}

// TestParseManifestsForeignRootPluginJSON is the regression guard: an unrelated
// root plugin.json must not be parsed, because a parse error flips a resolvable
// Plugin to Ready=False/SourceInvalid.
func TestParseManifestsForeignRootPluginJSON(t *testing.T) {
	b := &CanonicalBundle{Files: map[string][]byte{
		AgentPluginsManifestPath: []byte("{not json"),
		"skills/demo/SKILL.md":   []byte(skillFile),
	}}
	manifests, err := ParseManifests(b)
	if err != nil {
		t.Fatalf("foreign root plugin.json must not fail the parse, got: %v", err)
	}
	if manifests != nil {
		t.Errorf("got manifests %+v, want none", manifests)
	}
}

// TestParseManifestsMalformedRootPluginJSONIsSkipped pins a deliberate
// asymmetry. A malformed root plugin.json is SKIPPED, never an error, because
// unparseable bytes cannot tell us whether the file was ours or some other
// ecosystem's — and erroring on a foreign file would flip a resolvable Plugin
// to Ready=False/SourceInvalid.
//
// The cost is a real coverage hole: a genuinely broken agent-plugins manifest
// resolves as "no format detected" and is admitted by the gate's
// empty-formats-warn rule. The claude-plugin manifest still fails closed
// (TestParseManifest), because its path is unambiguous.
func TestParseManifestsMalformedRootPluginJSONIsSkipped(t *testing.T) {
	b := &CanonicalBundle{Files: map[string][]byte{
		AgentPluginsManifestPath: []byte(`{"$schema":"https://agent-plugins.org/schemas/plugin/1.0.0/plugin.schema.json","name":`),
	}}
	manifests, err := ParseManifests(b)
	if err != nil {
		t.Fatalf("malformed root plugin.json must be skipped, not fail: %v", err)
	}
	if manifests != nil {
		t.Errorf("got manifests %+v, want none", manifests)
	}
	if got := DetectFormats(b); got != nil {
		t.Errorf("DetectFormats() = %v, want none", got)
	}
}

func TestDetectMCPFiles(t *testing.T) {
	tests := []struct {
		name  string
		files map[string][]byte
		want  []string
	}{
		{"claude only", map[string][]byte{ClaudeMCPPath: []byte("{}")}, []string{ClaudeMCPPath}},
		{"agent-plugins only", map[string][]byte{AgentPluginsMCPPath: []byte("{}")}, []string{AgentPluginsMCPPath}},
		{
			"both",
			map[string][]byte{ClaudeMCPPath: []byte("{}"), AgentPluginsMCPPath: []byte("{}")},
			[]string{ClaudeMCPPath, AgentPluginsMCPPath},
		},
		{"neither", map[string][]byte{"README.md": []byte("hi")}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectMCPFiles(&CanonicalBundle{Files: tt.files})
			if !slices.Equal(got, tt.want) {
				t.Errorf("DetectMCPFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildInventoryMCPServerUnion(t *testing.T) {
	tests := []struct {
		name  string
		files map[string][]byte
		want  []string
	}{
		{
			name:  "claude only",
			files: map[string][]byte{ClaudeMCPPath: []byte(`{"mcpServers":{"b":{},"a":{}}}`)},
			want:  []string{"a", "b"},
		},
		{
			name:  "agent-plugins only",
			files: map[string][]byte{AgentPluginsMCPPath: []byte(`{"mcpServers":{"c":{}}}`)},
			want:  []string{"c"},
		},
		{
			name: "union deduplicates and sorts",
			files: map[string][]byte{
				ClaudeMCPPath:       []byte(`{"mcpServers":{"shared":{},"claude-only":{}}}`),
				AgentPluginsMCPPath: []byte(`{"mcpServers":{"shared":{},"ap-only":{}}}`),
			},
			want: []string{"ap-only", "claude-only", "shared"},
		},
		{
			name:  "malformed file is skipped, not fatal",
			files: map[string][]byte{ClaudeMCPPath: []byte("{nope"), AgentPluginsMCPPath: []byte(`{"mcpServers":{"a":{}}}`)},
			want:  []string{"a"},
		},
		{
			name:  "no mcp files",
			files: map[string][]byte{"README.md": []byte("hi")},
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildInventory(&CanonicalBundle{Files: tt.files}).MCPServers
			if !slices.Equal(got, tt.want) {
				t.Errorf("MCPServers = %v, want %v", got, tt.want)
			}
		})
	}
}
