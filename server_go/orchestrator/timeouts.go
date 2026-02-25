package orchestrator

import "time"

const runnerTaskTimeout = 20 * time.Minute

// RunnerTaskTimeout is the public alias for use outside the orchestrator package.
const RunnerTaskTimeout = runnerTaskTimeout
