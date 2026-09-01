package v1alpha1

// Plugin is the typed envelope for kind=Plugin resources.
//
// A Plugin is a self-contained, versioned bundle of harness extensions —
// skills, MCP servers, hooks, and sub-agents — modeled on the Claude Code
// plugin format. The Spec is USER INTENT ONLY: a pinned pointer to an external
// source (a git commit or — later — an OCI digest), the same source-based model
// agents and skills use. The registry hosts NOTHING; the Plugin controller
// resolves the pointer to a concrete commit/digest and scans the source for its
// manifest and inventory OUT OF BAND, recording that server-determined data in
// Status — never in Spec. The bundle is materialized from its source into a
// harness layout at deploy time.
type Plugin struct {
	TypeMeta `json:",inline" yaml:",inline"`
	Metadata ObjectMeta   `json:"metadata" yaml:"metadata"`
	Spec     PluginSpec   `json:"spec" yaml:"spec"`
	Status   PluginStatus `json:"status,omitzero" yaml:"status,omitempty"`
}

func init() {
	MustRegisterKind[*Plugin, PluginSpec](KindPlugin)
}

// PluginSpec is the plugin resource's declarative body — USER INTENT ONLY.
// Server-derived data (the resolved source pin, the parsed Manifest, and the
// derived Inventory) lives in PluginStatus, populated out of band by the Plugin
// controller. Keeping it out of the spec means a status write never changes the
// spec content hash, so re-applying identical intent is an UpsertNoOp.
type PluginSpec struct {
	Title       string `json:"title,omitempty" yaml:"title,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// IconURL is the image a catalog UI shows for this plugin. Either an
	// absolute https:// URL or a root-relative path served by the UI.
	IconURL string `json:"iconUrl,omitempty" yaml:"iconUrl,omitempty"`

	// Source is where the bundle is ingested from, pinned (git commit / OCI
	// digest) so a published tag is reproducible.
	Source *PluginSource `json:"source,omitempty" yaml:"source,omitempty"`
}

// PluginStatus is the Plugin observed-state subresource, written by the Plugin
// controller out of band of the API write. It embeds the shared Status
// (conditions + observedGeneration) and adds the server-determined resolution
// data.
//
// Readiness contract: consumers MUST treat the absence of a Ready=True condition
// (or ResolvedSource==nil) as "not yet resolved". The controller sets
// Ready=False/Reason=Progressing on first observe, Ready=True/Reason=Resolved
// once the pointer is pinned and the source scanned, and Ready=False with a
// specific reason (SourceUnresolvable, SourceUnsupported, SourceInvalid) on
// failure.
type PluginStatus struct {
	Status `json:",inline" yaml:",inline"`

	// ResolvedSource is the controller's immutable pin of the user's source
	// pointer (the concrete commit/digest the source resolved to).
	ResolvedSource *PluginResolvedSource `json:"resolvedSource,omitempty" yaml:"resolvedSource,omitempty"`
	// Formats are the bundle layouts the controller detected in the source,
	// sorted and deduplicated. A bundle may honestly be more than one format:
	// the agent-plugins spec permits third-party directories such as
	// .claude-plugin/, so a source shipping both manifests is both. An empty
	// list means nothing recognizable was found — that is a warning, not a
	// failure, and deploy-time gates must let it through.
	Formats []string `json:"formats,omitempty" yaml:"formats,omitempty"`
	// Manifests are the parsed manifests keyed by the format they were read
	// from (see PluginFormat*). A dual-format bundle records both, losslessly.
	Manifests map[string]*PluginManifest `json:"manifests,omitempty" yaml:"manifests,omitempty"`
	// MCPServerFiles are the MCP declaration files the bundle actually ships,
	// sorted — ".mcp.json" (claude-plugin) and/or "mcp.json" (agent-plugins).
	//
	// Inventory.MCPServers merges both files into one list, which is the right
	// answer for "what does this plugin declare" and the wrong one for "will
	// this target start them": a harness reads one filename, not both. Deploy
	// gates need the provenance the merged list discards, so it is recorded
	// separately rather than reconstructed.
	MCPServerFiles []string `json:"mcpServerFiles,omitempty" yaml:"mcpServerFiles,omitempty"`
	// Inventory is the server-derived risk surface / search index.
	Inventory *PluginInventory `json:"inventory,omitempty" yaml:"inventory,omitempty"`
}

// PreferredManifest returns the one manifest a single-manifest consumer should
// use, favouring claude-plugin because that is the format every current harness
// consumes. Returns nil when the bundle shipped no manifest.
//
// Prefer this over reading Manifests directly unless the caller genuinely cares
// which format it got.
func (s PluginStatus) PreferredManifest() *PluginManifest {
	for _, format := range []string{PluginFormatClaudePlugin, PluginFormatAgentPlugins} {
		if m, ok := s.Manifests[format]; ok {
			return m
		}
	}
	return nil
}

// Plugin bundle formats recorded in PluginStatus.Formats. Which formats a
// deploy target can actually consume is a deploy-time concern owned by the
// runtime adapters, not by this package.
const (
	// PluginFormatClaudePlugin is the Claude Code plugin layout: a
	// .claude-plugin/plugin.json manifest (optional — Claude auto-discovers
	// from skills/, commands/, agents/, hooks/hooks.json, .mcp.json) and MCP
	// servers in .mcp.json.
	PluginFormatClaudePlugin = "claude-plugin"
	// PluginFormatAgentPlugins is the Agent Plugins layout: a mandatory root
	// plugin.json carrying an agent-plugins $schema, and MCP servers in
	// mcp.json.
	PluginFormatAgentPlugins = "agent-plugins"
)

// MCP declaration filenames recorded in PluginStatus.MCPServerFiles. Part of
// the public status contract: deploy targets outside this repo compare against
// these to decide whether they will actually start a bundle's servers.
const (
	// PluginMCPFileClaudePlugin is where the claude-plugin format declares MCP
	// servers. Every current harness reads this file and only this file.
	PluginMCPFileClaudePlugin = ".mcp.json"
	// PluginMCPFileAgentPlugins is where the agent-plugins format declares MCP
	// servers. Same {"mcpServers": {…}} shape, different filename.
	PluginMCPFileAgentPlugins = "mcp.json"
)

// PluginResolvedSource records the concrete, immutable revision the controller
// pinned the user's source pointer to. Exactly one of Commit/Digest is set,
// matching Type. It is the reproducibility anchor: deploys materialize from this
// pin, not from the (possibly moving) ref the user supplied.
type PluginResolvedSource struct {
	Type PluginSourceType `json:"type" yaml:"type"`
	// Commit is the resolved full git commit SHA (Type=git).
	Commit string `json:"commit,omitempty" yaml:"commit,omitempty"`
	// Digest is the resolved OCI digest, e.g. "sha256:…" (Type=oci; future).
	Digest string `json:"digest,omitempty" yaml:"digest,omitempty"`
}

// PluginSourceType selects which source sub-struct is set.
type PluginSourceType string

const (
	PluginSourceTypeGit PluginSourceType = "git"
	PluginSourceTypeOCI PluginSourceType = "oci"
)

// PluginSource identifies where the bundle came from. Exactly one of Git/OCI
// is set, matching Type. The reference must be pinned (git commit / OCI digest)
// so the published tag is reproducible.
type PluginSource struct {
	Type PluginSourceType `json:"type" yaml:"type"`
	Git  *PluginSourceGit `json:"git,omitempty" yaml:"git,omitempty"`
	OCI  *PluginSourceOCI `json:"oci,omitempty" yaml:"oci,omitempty"`
}

// PluginSourceGit is a git source. Repository may pin a Commit, a Branch, or a
// tag (empty => the remote default branch); the Plugin controller resolves
// whatever ref is supplied to a concrete commit SHA and records that immutable
// pin in status.ResolvedSource. Repository.Subfolder selects a plugin inside a
// monorepo.
type PluginSourceGit struct {
	Repository *Repository `json:"repository" yaml:"repository"`
}

// PluginSourceOCI is a digest-pinned OCI artifact reference, e.g.
// "ghcr.io/org/plugin@sha256:...". Bare/tag-only refs are rejected.
type PluginSourceOCI struct {
	Reference string `json:"reference" yaml:"reference"`
}

// PluginInventory is the server-derived index of a bundle's actual contents,
// computed by scanning the bundle files (not the author-supplied manifest). It
// is the legible governance risk surface and the search index.
type PluginInventory struct {
	Skills   []PluginSkill `json:"skills,omitempty" yaml:"skills,omitempty"`
	Commands []string      `json:"commands,omitempty" yaml:"commands,omitempty"`
	// Agents are sub-agent names; sub-agents are markdown prompt files in the
	// bundle, not manifest entries.
	Agents []string `json:"agents,omitempty" yaml:"agents,omitempty"`
	// Hooks are lifecycle hooks the bundle registers (arbitrary code).
	Hooks      []PluginHook `json:"hooks,omitempty" yaml:"hooks,omitempty"`
	MCPServers []string     `json:"mcpServers,omitempty" yaml:"mcpServers,omitempty"`
	// Executables are bin/ entries the bundle ships (arbitrary code).
	Executables []string `json:"executables,omitempty" yaml:"executables,omitempty"`
}

// PluginSkill is one skill shipped in the bundle (from its SKILL.md frontmatter).
type PluginSkill struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// PluginHook is one lifecycle hook the bundle registers.
type PluginHook struct {
	// Event is the lifecycle event, e.g. "PreToolUse", "PostToolUse",
	// "SessionStart".
	Event string `json:"event" yaml:"event"`
	// Type is the handler kind: command|http|mcp_tool|prompt|agent.
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
}
