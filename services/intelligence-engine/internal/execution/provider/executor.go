package provider

import (
	"context"

	"github.com/atlas/intelligence-engine/internal/execution"
)

// ActionExecutor defines the strictly typed capabilities of the M2.7 execution engine.
// It avoids arbitrary strings like Execute("cmd").
type ActionExecutor interface {
	RestartService(ctx context.Context, serviceName string) execution.ExecutionResult
	Observe(ctx context.Context, serviceName string) execution.ExecutionResult
	Investigate(ctx context.Context, serviceName string) execution.ExecutionResult
}
