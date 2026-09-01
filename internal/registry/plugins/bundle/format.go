package bundle

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/agentregistry-dev/agentregistry/pkg/api/v1alpha1"
)

const (
	// ClaudeManifestPath is the manifest location of the claude-plugin format.
	// Claude treats the manifest as optional and auto-discovers components, so
	// its absence does not rule the format out (see DetectFormats).
	ClaudeManifestPath = ".claude-plugin/plugin.json"
	// AgentPluginsManifestPath is the manifest location of the agent-plugins
	// format. The agent-plugins spec makes the manifest mandatory, so its
	// absence does rule that format out.
	AgentPluginsManifestPath = "plugin.json"

	// ClaudeMCPPath is where the claude-plugin format declares MCP servers.
	ClaudeMCPPath = v1alpha1.PluginMCPFileClaudePlugin
	// AgentPluginsMCPPath is where the agent-plugins format declares MCP
	// servers. Same {"mcpServers": {…}} shape, different filename.
	AgentPluginsMCPPath = v1alpha1.PluginMCPFileAgentPlugins

	// agentPluginsSchemaPrefix identifies an agent-plugins manifest. Matched by
	// prefix, not against the exact versioned URL, so a spec version bump does
	// not silently stop detecting the format.
	agentPluginsSchemaPrefix = "https://agent-plugins.org/schemas/"
)

// claudeComponentDirs are the directories Claude auto-discovers when a bundle
// ships no manifest. Their presence is what makes a manifest-less bundle a
// claude-plugin bundle rather than an unrecognizable one.
var claudeComponentDirs = []string{"skills/", "commands/", "agents/"}

// claudeComponentFiles are the manifest-less signals that are files, not
// directories.
var claudeComponentFiles = []string{"hooks/hooks.json", ClaudeMCPPath}

// DetectFormats reports which bundle layouts the source ships, sorted and
// deduplicated. A bundle can honestly be more than one: the agent-plugins spec
// states that third-party directories such as .claude-plugin/ do not violate
// conformance, so a source carrying both manifests is both formats.
//
// An empty result means nothing recognizable was found. That is a warning, not
// a rejection — deploy-time gates must let it through, because refusing would
// break bundles that deploy fine today.
func DetectFormats(b *CanonicalBundle) []string {
	var formats []string
	if _, ok := b.Files[ClaudeManifestPath]; ok {
		formats = append(formats, v1alpha1.PluginFormatClaudePlugin)
	}
	if hasAgentPluginsFormat(b) {
		formats = append(formats, v1alpha1.PluginFormatAgentPlugins)
	}
	// Manifest-less fallback, and ONLY when neither manifest is present.
	// Claude's manifest is optional and it auto-discovers components, so a
	// bundle that is nothing but skills/ still loads and must stay deployable.
	//
	// Gating this on "no manifest at all" is load-bearing. An agent-plugins
	// bundle also ships skills/; if the fallback ran unconditionally, every
	// such bundle would also detect as claude-plugin, pass the harness gate on
	// that strength, and silently drop its mcp.json servers — the exact
	// half-materialization the gate exists to prevent.
	if len(formats) == 0 && hasClaudeComponents(b) {
		formats = append(formats, v1alpha1.PluginFormatClaudePlugin)
	}
	slices.Sort(formats)
	return formats
}

func hasClaudeComponents(b *CanonicalBundle) bool {
	for _, f := range claudeComponentFiles {
		if _, ok := b.Files[f]; ok {
			return true
		}
	}
	for p := range b.Files {
		for _, dir := range claudeComponentDirs {
			if strings.HasPrefix(p, dir) {
				return true
			}
		}
	}
	return false
}

// DetectMCPFiles reports which MCP declaration files the bundle ships, sorted.
// A harness reads one filename, so which file a server was declared in decides
// whether that harness will start it — a distinction the merged
// PluginInventory.MCPServers list cannot express.
func DetectMCPFiles(b *CanonicalBundle) []string {
	var found []string
	for _, p := range []string{ClaudeMCPPath, AgentPluginsMCPPath} {
		if _, ok := b.Files[p]; ok {
			found = append(found, p)
		}
	}
	slices.Sort(found)
	return found
}

// hasAgentPluginsFormat reports the agent-plugins layout: a root plugin.json
// carrying an agent-plugins $schema. The $schema test is the whole signal — a
// bare root plugin.json belongs to any number of unrelated ecosystems.
func hasAgentPluginsFormat(b *CanonicalBundle) bool {
	data, ok := b.Files[AgentPluginsManifestPath]
	return ok && isAgentPluginsManifest(data)
}

// isAgentPluginsManifest reports whether raw manifest bytes declare an
// agent-plugins $schema. Malformed JSON is not an agent-plugins manifest; it is
// deliberately not an error, so a foreign root plugin.json cannot flip a
// resolvable Plugin to Ready=False/SourceInvalid.
func isAgentPluginsManifest(data []byte) bool {
	var probe struct {
		Schema string `json:"$schema"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	return strings.HasPrefix(probe.Schema, agentPluginsSchemaPrefix)
}
