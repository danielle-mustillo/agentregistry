package commands

import (
	"reflect"
	"strings"
	"testing"

	"github.com/agentregistry-dev/agentregistry/pkg/api/v1alpha1"
)

func TestAgentDisplayMode(t *testing.T) {
	tests := []struct {
		name string
		spec v1alpha1.AgentSpec
		want string
	}{
		{
			name: "no runnable mode",
			want: "<none>",
		},
		{
			name: "empty source is not a runnable mode",
			spec: v1alpha1.AgentSpec{Source: &v1alpha1.AgentSource{}},
			want: "<none>",
		},
		{
			name: "image source",
			spec: v1alpha1.AgentSpec{
				Source: &v1alpha1.AgentSource{Image: "ghcr.io/example/agent:v1"},
			},
			want: "source",
		},
		{
			name: "repository source",
			spec: v1alpha1.AgentSpec{
				Source: &v1alpha1.AgentSource{
					Repository: &v1alpha1.Repository{URL: "https://github.com/example/agent"},
				},
			},
			want: "source",
		},
		{
			name: "plugin ref needs a harness",
			spec: v1alpha1.AgentSpec{
				Plugins: []v1alpha1.ResourceRef{{Name: "reviewer"}},
			},
			want: "harness",
		},
		{
			name: "skill ref needs a harness",
			spec: v1alpha1.AgentSpec{
				Skills: []v1alpha1.ResourceRef{{Name: "summarize"}},
			},
			want: "harness",
		},
		{
			name: "instructions ref needs a harness",
			spec: v1alpha1.AgentSpec{
				Instructions: &v1alpha1.ResourceRef{Name: "system-prompt"},
			},
			want: "harness",
		},
		{
			name: "source and composition refs",
			spec: v1alpha1.AgentSpec{
				Source:  &v1alpha1.AgentSource{Image: "ghcr.io/example/agent:v1"},
				Plugins: []v1alpha1.ResourceRef{{Name: "reviewer"}},
			},
			want: "source+harness",
		},
		{
			// MCPServers flow to any MCP-capable runtime, harness or not, so
			// they are the one composition-adjacent ref that does not imply a
			// harness. Reporting "harness" here would send a BYO agent author
			// looking for one they do not need.
			name: "mcp server refs alone do not need a harness",
			spec: v1alpha1.AgentSpec{
				Source:     &v1alpha1.AgentSource{Image: "ghcr.io/example/agent:v1"},
				MCPServers: []v1alpha1.ResourceRef{{Name: "github"}},
			},
			want: "source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := agentDisplayMode(tt.spec); got != tt.want {
				t.Fatalf("agentDisplayMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAgentRowIncludesModeAndDescription(t *testing.T) {
	agent := &v1alpha1.Agent{
		Metadata: v1alpha1.ObjectMeta{Name: "reviewer", Tag: "stable"},
		Spec: v1alpha1.AgentSpec{
			Description: "Reviews pull requests",
			Source:      &v1alpha1.AgentSource{Image: "ghcr.io/example/reviewer:v1"},
			Skills:      []v1alpha1.ResourceRef{{Name: "code-review"}},
		},
	}

	want := []string{"reviewer", "stable", "source+harness", "Reviews pull requests"}
	if got := agentRow(agent); !reflect.DeepEqual(got, want) {
		t.Fatalf("agentRow() = %#v, want %#v", got, want)
	}
}

func TestSecretRowShowsMetadataWithoutPayloadValues(t *testing.T) {
	secret := &v1alpha1.Secret{
		Metadata: v1alpha1.ObjectMeta{Name: "provider-credentials"},
		Spec: v1alpha1.SecretSpec{
			Immutable:  true,
			Data:       map[string]string{"token": "c2Vuc2l0aXZl"},
			StringData: map[string]string{"password": "sensitive"},
		},
		Status: v1alpha1.SecretStatus{DataKeys: []string{"password", "token"}},
	}

	want := []string{"provider-credentials", "Opaque", "password,token", "true"}
	got := secretRow(secret)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("secretRow() = %#v, want %#v", got, want)
	}
	row := strings.Join(got, " ")
	for _, payload := range []string{"c2Vuc2l0aXZl", "sensitive"} {
		if strings.Contains(row, payload) {
			t.Fatalf("secretRow() exposed payload value %q", payload)
		}
	}
}
