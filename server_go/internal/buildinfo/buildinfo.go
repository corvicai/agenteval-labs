package buildinfo

// These vars are injected at build time via -ldflags:
//
//	-X benchmarking-platform/internal/buildinfo.Commit=<sha>
//	-X benchmarking-platform/internal/buildinfo.Branch=<branch>
//	-X benchmarking-platform/internal/buildinfo.Dirty=<dirty>
//	-X benchmarking-platform/internal/buildinfo.UpdatedAt=<timestamp>
//
// They fall back to empty strings if not injected.
var (
	Commit    = ""
	Branch    = ""
	Dirty     = ""
	UpdatedAt = ""
)
