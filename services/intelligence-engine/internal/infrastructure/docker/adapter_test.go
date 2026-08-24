//go:build !windows

package docker

import (
	"context"
	"errors"
	"testing"

	"github.com/atlas/intelligence-engine/internal/execution"
	"github.com/docker/docker/api/types/container"
)

// fakeRestarter is a test double for the Docker SDK client, injected through
// the ContainerRestarter seam so the adapter's control flow can be verified
// without a live Docker daemon. The real production path (NewAdapter) is
// untouched -- it still constructs and uses the real *client.Client.
type fakeRestarter struct {
	restartedContainer string
	callCount          int
	err                error
}

func (f *fakeRestarter) ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error {
	f.callCount++
	f.restartedContainer = containerID
	return f.err
}

func TestRestartService_UnknownServiceIsRejectedWithoutCallingDocker(t *testing.T) {
	fr := &fakeRestarter{}
	a := &Adapter{cli: fr}

	res := a.RestartService(context.Background(), "not-a-real-service")

	if res.Status != execution.StatusRejected {
		t.Fatalf("expected StatusRejected for an unmapped service, got %s", res.Status)
	}
	if fr.callCount != 0 {
		t.Fatalf("expected the Docker client to never be called for an unmapped service, got %d calls", fr.callCount)
	}
}

func TestRestartService_MapsToCorrectContainerName(t *testing.T) {
	for service, wantContainer := range execution.AllowedServices {
		fr := &fakeRestarter{}
		a := &Adapter{cli: fr}

		res := a.RestartService(context.Background(), service)

		if res.Status != execution.StatusExecuted {
			t.Fatalf("%s: expected StatusExecuted, got %s (%s)", service, res.Status, res.Message)
		}
		if fr.restartedContainer != wantContainer {
			t.Fatalf("%s: expected container %q to be restarted, got %q", service, wantContainer, fr.restartedContainer)
		}
	}
}

func TestRestartService_PropagatesInfrastructureFailure(t *testing.T) {
	fr := &fakeRestarter{err: errors.New("docker daemon unreachable")}
	a := &Adapter{cli: fr}

	res := a.RestartService(context.Background(), "atlas-payment-service")

	if res.Status != execution.StatusFailed {
		t.Fatalf("expected StatusFailed when the Docker client errors, got %s", res.Status)
	}
	if res.Error == nil {
		t.Fatal("expected the underlying Docker error to be propagated, got nil")
	}
}

func TestObserveAndInvestigate_SucceedWithoutTouchingDocker(t *testing.T) {
	fr := &fakeRestarter{}
	a := &Adapter{cli: fr}

	if res := a.Observe(context.Background(), "atlas-payment-service"); res.Status != execution.StatusExecuted {
		t.Fatalf("Observe: expected StatusExecuted, got %s", res.Status)
	}
	if res := a.Investigate(context.Background(), "atlas-payment-service"); res.Status != execution.StatusExecuted {
		t.Fatalf("Investigate: expected StatusExecuted, got %s", res.Status)
	}
	if fr.callCount != 0 {
		t.Fatalf("Observe/Investigate must never call the Docker client, got %d calls", fr.callCount)
	}
}
