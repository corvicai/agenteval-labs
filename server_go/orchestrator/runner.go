package orchestrator

import "context"

type Runner interface {
	Execute(ctx context.Context, req ExecutionRequest) (ExecutionResponse, error)
	Health() error
}

func newRunner() Runner {
	return newGoRunner()
}
