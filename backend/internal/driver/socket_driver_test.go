package driver

import (
	"context"
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
