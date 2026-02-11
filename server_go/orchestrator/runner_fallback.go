package orchestrator

import (
	"context"
	"log"
	"strings"
)

type fallbackRunner struct {
	primary  Runner
	fallback Runner
}

func newFallbackRunner(primary Runner, fallback Runner) Runner {
	return &fallbackRunner{primary: primary, fallback: fallback}
}

func (r *fallbackRunner) Health() error {
	if r.primary == nil {
		return nil
	}
	return r.primary.Health()
}

func (r *fallbackRunner) Execute(ctx context.Context, req ExecutionRequest) (ExecutionResponse, error) {
	if r.primary == nil {
		if r.fallback == nil {
			return ExecutionResponse{Success: false, Error: "runner not configured"}, nil
		}
		return r.fallback.Execute(ctx, req)
	}

	res, err := r.primary.Execute(ctx, req)
	if ctx != nil && ctx.Err() != nil {
		return res, err
	}
	if !shouldFallback(res, err) || r.fallback == nil {
		return res, err
	}

	log.Printf("[RUNNER] Primary runner failed, falling back to HTTP runner")
	return r.fallback.Execute(ctx, req)
}

func shouldFallback(res ExecutionResponse, err error) bool {
	if err != nil {
		return true
	}
	if res.Success {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(res.Error))
	if msg == "" {
		return false
	}

	// Don't fallback on known configuration/user errors.
	skip := []string{
		"missing", "unknown provider type", "mock execution is disabled", "api key is missing",
	}
	for _, s := range skip {
		if strings.Contains(msg, s) {
			return false
		}
	}

	// Fallback on transport/runtime style failures.
	keywords := []string{
		"connection refused",
		"dial tcp",
		"no such host",
		"context deadline",
		"timeout",
		"connection reset",
		"eof",
		"tls",
		"handshake",
		"temporary",
		"unavailable",
		"stream",
		"session",
		"status 5",
		"502",
		"503",
		"504",
		"unable to extract response text",
		"openai response missing",
	}
	for _, k := range keywords {
		if strings.Contains(msg, k) {
			return true
		}
	}
	return false
}
