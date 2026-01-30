// Package handler provides command handlers for the v2 orchestration architecture.
// This file re-exports error sentinels from types and repository packages for backward compatibility.
package handler

import (
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/orchestration/v2/types"
)

// Process lifecycle errors
var (
	ErrProcessNotFound     = repository.ErrProcessNotFound // From repository (can't be in types due to import cycle)
	ErrProcessRetired      = types.ErrProcessRetired
	ErrCoordinatorExists   = types.ErrCoordinatorExists
	ErrObserverExists      = types.ErrObserverExists
	ErrMaxProcessesReached = types.ErrMaxProcessesReached
	ErrNotSpawning         = types.ErrNotSpawning
	ErrAlreadyRetired      = types.ErrAlreadyRetired
)

// Queue errors
var (
	ErrQueueEmpty = types.ErrQueueEmpty
)

// State transition errors
var (
	ErrInvalidPhase = types.ErrInvalidPhaseTransition
)
