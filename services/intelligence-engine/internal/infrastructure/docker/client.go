//go:build !windows

package docker

import (
	"context"

	"github.com/docker/docker/api/types/container"
)

// ContainerRestarter is the minimal subset of the Docker SDK client the
// Adapter depends on. It exists purely so tests can inject a fake
// implementation without a live Docker daemon; the production path still
// constructs and uses the real *client.Client from the Docker SDK, unchanged.
type ContainerRestarter interface {
	ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error
}
