package orchestrator

import "context"

type Runner interface {
	Execute(ctx context.Context, req ExecutionRequest) (ExecutionResponse, error)
	Health() error
}

func newRunner() Runner {
	return newGoRunner()
}

// NewGoRunner creates a new runner instance for use outside the orchestrator package.
func NewGoRunner() Runner {
	return newGoRunner()
}
