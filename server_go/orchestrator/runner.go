package orchestrator

import (
	"context"
	"os"
	"strings"
)

type Runner interface {
	Execute(ctx context.Context, req ExecutionRequest) (ExecutionResponse, error)
	Health() error
}

func newRunner(pythonURL string) Runner {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("RUNNER_MODE")))
	if mode == "" {
		mode = "go"
	}

	trimmedURL := strings.TrimSpace(pythonURL)
	if mode == "http" || mode == "python" || mode == "external" {
		if trimmedURL == "" {
			trimmedURL = "http://localhost:3003"
		}
		return newHTTPRunner(trimmedURL)
	}

	return newGoRunner()
}

func shouldUseGoRunner(pythonURL string) bool { return strings.TrimSpace(pythonURL) == "" }
