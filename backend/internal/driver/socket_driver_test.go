package driver

import (
	"context"
	"strings"
	"testing"
)

func TestPathTranslation(t *testing.T) {
	d := &SocketDriver{
		mounts: []hostMount{
			{
				Source:      "/home/farmers00/docker",
				Destination: "/root/docker",
			},
		},
		mountsInit: true,
	}

	ctx := context.Background()

	tests := []struct {
		name              string
		input             string
		expectedHost      string
		expectedContainer string
	}{
		{
			name:              "Container path to host",
			input:             "/root/docker/myapp",
			expectedHost:      "/home/farmers00/docker/myapp",
			expectedContainer: "/root/docker/myapp",
		},
		{
			name:              "Host path to container",
			input:             "/home/farmers00/docker/myapp",
			expectedHost:      "/home/farmers00/docker/myapp",
			expectedContainer: "/root/docker/myapp",
		},
		{
			name:              "Base container dir",
			input:             "/root/docker",
			expectedHost:      "/home/farmers00/docker",
			expectedContainer: "/root/docker",
		},
		{
			name:              "Base host dir",
			input:             "/home/farmers00/docker",
			expectedHost:      "/home/farmers00/docker",
			expectedContainer: "/root/docker",
		},
		{
			name:              "Nested subdir in container",
			input:             "/root/docker/stacks/web/v1",
			expectedHost:      "/home/farmers00/docker/stacks/web/v1",
			expectedContainer: "/root/docker/stacks/web/v1",
		},
		{
			name:              "Unrelated path",
			input:             "/etc/nginx",
			expectedHost:      "/etc/nginx",
			expectedContainer: "/etc/nginx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost := d.ToHostPath(ctx, tt.input)
			if gotHost != tt.expectedHost {
				t.Errorf("ToHostPath(%q) = %q; want %q", tt.input, gotHost, tt.expectedHost)
			}

			gotContainer := d.ToContainerPath(ctx, tt.input)
			if gotContainer != tt.expectedContainer {
				t.Errorf("ToContainerPath(%q) = %q; want %q", tt.input, gotContainer, tt.expectedContainer)
			}
		})
	}
}

func TestSanitizeCompose(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantMod     bool
		wantOld     string
		wantNew     string
		containsStr string
	}{
		{
			name:        "Uppercase top-level name",
			input:       "name: Ollama\nservices:\n  ollama:\n    image: ollama/ollama\n",
			wantMod:     true,
			wantOld:     "Ollama",
			wantNew:     "ollama",
			containsStr: "name: ollama\n",
		},
		{
			name:        "User case - name with space without quotes",
			input:       "name: ollama stack\nservices:\n  open-webui:\n    image: ghcr.io/open-webui/open-webui:cuda\n",
			wantMod:     true,
			wantOld:     "ollama stack",
			wantNew:     "ollama-stack",
			containsStr: "name: ollama-stack\n",
		},
		{
			name:        "Name with space and quotes",
			input:       "name: \"Ollama LLM\" # my server\nservices:\n  web:\n    image: nginx\n",
			wantMod:     true,
			wantOld:     "Ollama LLM",
			wantNew:     "ollama-llm",
			containsStr: "name: ollama-llm # my server\n",
		},
		{
			name:        "Already valid name",
			input:       "name: ollama\nservices:\n  ollama:\n    image: ollama/ollama\n",
			wantMod:     false,
			wantOld:     "",
			wantNew:     "",
			containsStr: "name: ollama\n",
		},
		{
			name:        "No top-level name",
			input:       "version: '3.8'\nservices:\n  ollama:\n    name: test\n",
			wantMod:     false,
			wantOld:     "",
			wantNew:     "",
			containsStr: "version: '3.8'\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newContent, mod, oldN, newN := sanitizeComposeContent(tt.input)
			if mod != tt.wantMod {
				t.Errorf("sanitizeComposeContent() modified = %v; want %v", mod, tt.wantMod)
			}
			if oldN != tt.wantOld {
				t.Errorf("sanitizeComposeContent() oldName = %q; want %q", oldN, tt.wantOld)
			}
			if newN != tt.wantNew {
				t.Errorf("sanitizeComposeContent() newName = %q; want %q", newN, tt.wantNew)
			}
			if !strings.Contains(newContent, tt.containsStr) {
				t.Errorf("sanitizeComposeContent() result %q does not contain %q", newContent, tt.containsStr)
			}
		})
	}
}
