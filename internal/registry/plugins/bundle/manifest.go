package bundle

import (
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/agentregistry-dev/agentregistry/pkg/api/v1alpha1"
)

// ParseManifests parses every manifest the bundle ships into the typed,
// faithful PluginManifest, keyed by the format it was read from. A dual-format
// bundle yields both entries, losslessly. Returns (nil, nil) when the bundle
// ships no manifest, or (nil, err) when a manifest is present but malformed
// (the caller decides whether to fail).
//
// The agent-plugins manifest is read only when its $schema identifies it as
// one. Unguarded, an unrelated root plugin.json — common in other ecosystems —
// would fail to parse and flip a resolvable Plugin to Ready=False/SourceInvalid.
func ParseManifests(b *CanonicalBundle) (map[string]*v1alpha1.PluginManifest, error) {
	out := map[string]*v1alpha1.PluginManifest{}
	if data, ok := b.Files[ClaudeManifestPath]; ok {
		m, err := unmarshalManifest(data, ClaudeManifestPath)
		if err != nil {
			return nil, err
		}
		out[v1alpha1.PluginFormatClaudePlugin] = m
	}
	if data, ok := b.Files[AgentPluginsManifestPath]; ok && isAgentPluginsManifest(data) {
		m, err := unmarshalManifest(data, AgentPluginsManifestPath)
		if err != nil {
			return nil, err
		}
		out[v1alpha1.PluginFormatAgentPlugins] = m
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// ParseManifest returns the single preferred manifest, favouring the
// claude-plugin location when the bundle ships both.
//
// Deprecated: use ParseManifests, which is lossless for dual-format bundles.
func ParseManifest(b *CanonicalBundle) (*v1alpha1.PluginManifest, error) {
	manifests, err := ParseManifests(b)
	if err != nil {
		return nil, err
	}
	return v1alpha1.PluginStatus{Manifests: manifests}.PreferredManifest(), nil
}

func unmarshalManifest(data []byte, atPath string) (*v1alpha1.PluginManifest, error) {
	var m v1alpha1.PluginManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%w: parse %s: %v", ErrInvalidBundle, atPath, err)
	}
	return &m, nil
}

// BuildInventory indexes a canonical bundle into a PluginInventory: the skills,
// sub-agents, commands, MCP servers, hooks, and bin/ executables it actually
// ships — the legible governance risk surface, derived by scanning bundle files
// (not the author-supplied manifest). Best-effort: a malformed declarative file
// is skipped rather than failing the resolve. Output is deterministic (sorted).
func BuildInventory(b *CanonicalBundle) *v1alpha1.PluginInventory {
	m := &v1alpha1.PluginInventory{}

	for _, p := range slices.Sorted(maps.Keys(b.Files)) {
		switch {
		case p == "SKILL.md" || (strings.HasPrefix(p, "skills/") && strings.HasSuffix(p, "/SKILL.md")):
			name, desc := parseSkillFrontmatter(b.Files[p])
			if name == "" {
				name = skillNameFromPath(p)
			}
			m.Skills = append(m.Skills, v1alpha1.PluginSkill{Name: name, Description: desc})
		case strings.HasPrefix(p, "agents/") && strings.HasSuffix(p, ".md"):
			m.Agents = append(m.Agents, baseNameNoExt(p))
		case strings.HasPrefix(p, "commands/") && strings.HasSuffix(p, ".md"):
			m.Commands = append(m.Commands, baseNameNoExt(p))
		case strings.HasPrefix(p, "bin/") && p != "bin/":
			m.Executables = append(m.Executables, strings.TrimPrefix(p, "bin/"))
		}
	}
	m.MCPServers = buildMCPServerUnion(b)
	if data, ok := b.Files["hooks/hooks.json"]; ok {
		m.Hooks = parseHooks(data)
	}
	return m
}

// parseSkillFrontmatter extracts name/description from a SKILL.md YAML
// frontmatter block (--- ... ---). Returns empties on any parse failure.
func parseSkillFrontmatter(content []byte) (name, desc string) {
	s := string(content)
	if !strings.HasPrefix(s, "---") {
		return "", ""
	}
	rest := s[3:]
	frontmatter, _, ok := strings.Cut(rest, "\n---")
	if !ok {
		return "", ""
	}
	var meta struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &meta); err != nil {
		return "", ""
	}
	return meta.Name, meta.Description
}

// buildMCPServerUnion returns the deduplicated, sorted union of the server
// names declared in both MCP files: .mcp.json (claude-plugin) and mcp.json
// (agent-plugins). The inventory reports what the bundle DECLARES; whether a
// given deploy target will actually start those servers is the deploy-time
// gate's question, not the inventory's.
func buildMCPServerUnion(b *CanonicalBundle) []string {
	seen := map[string]bool{}
	var names []string
	for _, p := range []string{ClaudeMCPPath, AgentPluginsMCPPath} {
		data, ok := b.Files[p]
		if !ok {
			continue
		}
		for _, name := range parseMCPServers(data) {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)
	return names
}

// parseMCPServers returns the sorted server names declared in an MCP file.
func parseMCPServers(data []byte) []string {
	var doc struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	names := make([]string, 0, len(doc.MCPServers))
	for k := range doc.MCPServers {
		names = append(names, k)
	}
	slices.Sort(names)
	return names
}

// parseHooks flattens a hooks.json ({hooks:{<Event>:[{hooks:[{type}]}]}}) into
// a deduplicated, sorted list of (event, handler-type) pairs.
func parseHooks(data []byte) []v1alpha1.PluginHook {
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type string `json:"type"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	events := make([]string, 0, len(doc.Hooks))
	for ev := range doc.Hooks {
		events = append(events, ev)
	}
	slices.Sort(events)

	seen := map[string]bool{}
	var out []v1alpha1.PluginHook
	for _, ev := range events {
		for _, matcher := range doc.Hooks[ev] {
			if len(matcher.Hooks) == 0 {
				if key := ev + "|"; !seen[key] {
					seen[key] = true
					out = append(out, v1alpha1.PluginHook{Event: ev})
				}
				continue
			}
			for _, h := range matcher.Hooks {
				if key := ev + "|" + h.Type; !seen[key] {
					seen[key] = true
					out = append(out, v1alpha1.PluginHook{Event: ev, Type: h.Type})
				}
			}
		}
	}
	return out
}

func baseNameNoExt(p string) string {
	b := path.Base(p)
	return strings.TrimSuffix(b, path.Ext(b))
}

func skillNameFromPath(p string) string {
	if strings.HasPrefix(p, "skills/") && strings.HasSuffix(p, "/SKILL.md") {
		mid := strings.TrimSuffix(strings.TrimPrefix(p, "skills/"), "/SKILL.md")
		if name, _, ok := strings.Cut(mid, "/"); ok {
			return name
		}
		return mid
	}
	return ""
}
