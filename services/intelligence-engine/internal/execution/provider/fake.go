package provider

import (
	"context"
	"errors"

	"github.com/atlas/intelligence-engine/internal/execution"
)

type FakeExecutor struct {
	ShouldFail bool
}

func NewFakeExecutor() *FakeExecutor {
	return &FakeExecutor{}
}

func (f *FakeExecutor) RestartService(ctx context.Context, serviceName string) execution.ExecutionResult {
	if f.ShouldFail {
		return execution.ExecutionResult{
			Status:  execution.StatusFailed,
			Message: "Fake failure",
			Error:   errors.New("simulated infrastructure failure"),
		}
	}
	return execution.ExecutionResult{
		Status:  execution.StatusExecuted,
		Message: "Simulated restart of " + serviceName + " completed",
		Error:   nil,
	}
}

func (f *FakeExecutor) Observe(ctx context.Context, serviceName string) execution.ExecutionResult {
	return execution.ExecutionResult{
		Status:  execution.StatusExecuted,
		Message: "Observed " + serviceName,
		Error:   nil,
	}
}

func (f *FakeExecutor) Investigate(ctx context.Context, serviceName string) execution.ExecutionResult {
	return execution.ExecutionResult{
		Status:  execution.StatusExecuted,
		Message: "Investigated " + serviceName,
		Error:   nil,
	}
}
