package dashboard

import (
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	controlplanemocks "github.com/zjrosen/perles/internal/orchestration/controlplane/mocks"
)

// newMockControlPlane creates a mockery-generated MockControlPlane with default expectations.
// The mock includes a default GetHealthStatus expectation that returns healthy status.
// Individual tests can override this with more specific expectations using EXPECT().
func newMockControlPlane(t *testing.T) *controlplanemocks.MockControlPlane {
	m := controlplanemocks.NewMockControlPlane(t)
	// Default mock for GetHealthStatus - returns healthy status
	// Individual tests can override with more specific expectations
	m.EXPECT().GetHealthStatus(mock.Anything).Return(controlplane.HealthStatus{
		IsHealthy: true,
	}, true).Maybe()
	return m
}
