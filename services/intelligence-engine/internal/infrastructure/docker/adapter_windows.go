//go:build windows

package docker

import (
	"context"
	"errors"

	"github.com/atlas/intelligence-engine/internal/execution"
)

type Adapter struct{}

func NewAdapter() (*Adapter, error) {
	return nil, errors.New("Docker adapter is not supported natively on Windows due to SDK limitations. Compile with GOOS=linux")
}

func (a *Adapter) RestartService(ctx context.Context, serviceName string) execution.ExecutionResult {
	return execution.ExecutionResult{
		Status:  execution.StatusFailed,
		Message: "Windows native execution not supported",
		Error:   errors.New("not supported"),
	}
}

func (a *Adapter) Observe(ctx context.Context, serviceName string) execution.ExecutionResult {
	return execution.ExecutionResult{Status: execution.StatusFailed, Error: errors.New("not supported")}
}

func (a *Adapter) Investigate(ctx context.Context, serviceName string) execution.ExecutionResult {
	return execution.ExecutionResult{Status: execution.StatusFailed, Error: errors.New("not supported")}
}
