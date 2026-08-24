//go:build !windows

package docker

import (
	"context"
	"errors"
	"log"

	"github.com/atlas/intelligence-engine/internal/execution"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type Adapter struct {
	cli *client.Client
}

func NewAdapter() (*Adapter, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Adapter{cli: cli}, nil
}

func (a *Adapter) RestartService(ctx context.Context, serviceName string) execution.ExecutionResult {
	// The Guard has already validated that serviceName is in the allowlist.
	// We do one final strict mapping to ensure we only restart explicitly known targets.
	containerName, ok := execution.AllowedServices[serviceName]
	if !ok {
		return execution.ExecutionResult{
			Status:  execution.StatusRejected,
			Message: "Unknown service target mapping",
			Error:   errors.New("service is not explicitly mapped to a container"),
		}
	}

	log.Printf("[INFO] Restarting strictly mapped Docker container: %s", containerName)
	err := a.cli.ContainerRestart(ctx, containerName, container.StopOptions{})
	if err != nil {
		return execution.ExecutionResult{
			Status:  execution.StatusFailed,
			Message: "Infrastructure operation failed",
			Error:   err,
		}
	}

	return execution.ExecutionResult{
		Status:  execution.StatusExecuted,
		Message: "Successfully restarted container " + containerName,
		Error:   nil,
	}
}

func (a *Adapter) Observe(ctx context.Context, serviceName string) execution.ExecutionResult {
	return execution.ExecutionResult{
		Status:  execution.StatusExecuted,
		Message: "Observed " + serviceName,
		Error:   nil,
	}
}

func (a *Adapter) Investigate(ctx context.Context, serviceName string) execution.ExecutionResult {
	return execution.ExecutionResult{
		Status:  execution.StatusExecuted,
		Message: "Investigated " + serviceName,
		Error:   nil,
	}
}
