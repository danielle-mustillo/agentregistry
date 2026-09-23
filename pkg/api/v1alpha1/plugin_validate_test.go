package v1alpha1

import (
	"strings"
	"testing"
)

func basePluginMeta() ObjectMeta {
	return ObjectMeta{Namespace: "default", Name: "my-plugin", Tag: "v1"}
}

func TestPluginValidate_IconURL(t *testing.T) {
	// Source is required, so every case carries an otherwise-valid one.
	source := &PluginSource{
		Type: PluginSourceTypeGit,
		Git:  &PluginSourceGit{Repository: &Repository{URL: "https://github.com/org/repo", Branch: "main"}},
	}
	for _, tc := range iconURLCases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Plugin{
				TypeMeta: TypeMeta{APIVersion: GroupVersion, Kind: KindPlugin},
				Metadata: basePluginMeta(),
				Spec:     PluginSpec{IconURL: tc.iconURL, Type: PluginTypeClaudePlugin, Source: source},
			}
			err := p.Validate()
			switch {
			case !tc.wantErr && err != nil:
				t.Fatalf("iconUrl %q: expected valid, got: %v", tc.iconURL, err)
			case tc.wantErr && err == nil:
				t.Fatalf("iconUrl %q: expected an error, got nil", tc.iconURL)
			case tc.wantErr && !strings.Contains(err.Error(), "spec.iconUrl"):
				t.Fatalf("iconUrl %q: error %q does not mention spec.iconUrl", tc.iconURL, err.Error())
			}
		})
	}
}

func TestPluginValidate(t *testing.T) {
	fullSHA := strings.Repeat("a1b2c3d4", 5) // 40 hex chars
	gitPinned := &PluginSource{
		Type: PluginSourceTypeGit,
		Git:  &PluginSourceGit{Repository: &Repository{URL: "https://github.com/org/repo", Commit: fullSHA}},
	}

	tests := []struct {
		name    string
		spec    PluginSpec
		wantErr string // substring; empty means valid
	}{
		{
			name: "valid git source",
			spec: PluginSpec{Title: "My Plugin", Type: PluginTypeClaudePlugin, Source: gitPinned},
		},
		{
			name: "valid oci digest source",
			spec: PluginSpec{Type: PluginTypeClaudePlugin, Source: &PluginSource{Type: PluginSourceTypeOCI, OCI: &PluginSourceOCI{Reference: "ghcr.io/org/plugin@sha256:" + strings.Repeat("a", 64)}}},
		},
		{
			name:    "missing source",
			spec:    PluginSpec{Type: PluginTypeClaudePlugin, Title: "x"},
			wantErr: "spec.source",
		},
		{
			name: "git source with branch only (controller resolves the commit)",
			spec: PluginSpec{Type: PluginTypeClaudePlugin, Source: &PluginSource{Type: PluginSourceTypeGit, Git: &PluginSourceGit{Repository: &Repository{URL: "https://github.com/org/repo", Branch: "main"}}}},
		},
		{
			name: "git source with no ref (controller resolves default branch)",
			spec: PluginSpec{Type: PluginTypeClaudePlugin, Source: &PluginSource{Type: PluginSourceTypeGit, Git: &PluginSourceGit{Repository: &Repository{URL: "https://github.com/org/repo"}}}},
		},
		{
			name:    "git source missing url",
			spec:    PluginSpec{Type: PluginTypeClaudePlugin, Source: &PluginSource{Type: PluginSourceTypeGit, Git: &PluginSourceGit{Repository: &Repository{Commit: fullSHA}}}},
			wantErr: "url",
		},
		{
			name:    "git commit not a full SHA (would never resolve)",
			spec:    PluginSpec{Type: PluginTypeClaudePlugin, Source: &PluginSource{Type: PluginSourceTypeGit, Git: &PluginSourceGit{Repository: &Repository{URL: "https://github.com/org/repo", Commit: "abc123"}}}},
			wantErr: "full 40-character SHA",
		},
		{
			name:    "git branch and commit both set (ambiguous)",
			spec:    PluginSpec{Type: PluginTypeClaudePlugin, Source: &PluginSource{Type: PluginSourceTypeGit, Git: &PluginSourceGit{Repository: &Repository{URL: "https://github.com/org/repo", Branch: "main", Commit: fullSHA}}}},
			wantErr: "at most one of branch or commit",
		},
		{
			name:    "oci source not digest-pinned",
			spec:    PluginSpec{Type: PluginTypeClaudePlugin, Source: &PluginSource{Type: PluginSourceTypeOCI, OCI: &PluginSourceOCI{Reference: "ghcr.io/org/plugin:latest"}}},
			wantErr: "digest-pinned",
		},
		{
			name:    "unknown source type",
			spec:    PluginSpec{Type: PluginTypeClaudePlugin, Source: &PluginSource{Type: "svn"}},
			wantErr: "unknown plugin source type",
		},
		{
			name:    "missing type",
			spec:    PluginSpec{Source: gitPinned},
			wantErr: "spec.type",
		},
		{
			name: "agent-plugins type",
			spec: PluginSpec{Type: PluginTypeAgentPlugins, Source: gitPinned},
		},
		{
			name:    "unknown type",
			spec:    PluginSpec{Type: "codex-plugin", Source: gitPinned},
			wantErr: "spec.type",
		},
		{
			name:    "git and oci both set",
			spec:    PluginSpec{Type: PluginTypeClaudePlugin, Source: &PluginSource{Type: PluginSourceTypeGit, Git: &PluginSourceGit{Repository: &Repository{URL: "https://github.com/org/repo", Commit: "abc"}}, OCI: &PluginSourceOCI{Reference: "x"}}},
			wantErr: "oci must be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plugin{
				TypeMeta: TypeMeta{APIVersion: GroupVersion, Kind: KindPlugin},
				Metadata: basePluginMeta(),
				Spec:     tt.spec,
			}
			err := p.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
