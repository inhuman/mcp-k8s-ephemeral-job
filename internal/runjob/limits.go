package runjob

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
)

// ResolvedResources holds the pod resources normalized into valid k8s quantity strings.
//
// Semantics (important): the server defaults are REQUESTS (a small scheduler
// reservation, so the pod fits on any node), not limits. Limits are set ONLY when the
// caller passed `limits` explicitly — otherwise the field stays empty and the namespace
// LimitRange supplies the ceiling via its default. An earlier version put the defaults
// into the container's limits; k8s then copies them into requests, the LimitRange
// default never applies, every job lives caged in the defaults, and a generous
// LimitRange is dead configuration.
type ResolvedResources struct {
	CPURequest    string
	MemoryRequest string
	CPULimit      string // "" = leave unset (the LimitRange default supplies the ceiling)
	MemoryLimit   string // "" = leave unset
}

// ResolveResources applies the requests/limits semantics and checks that the values
// parse as k8s quantities. When the caller sets limits.memory, the memory request is
// raised to match: memory is incompressible, so a pod that may really consume N must
// be scheduled onto a node that has N (a limit above what the node has free means an
// OOM for the node and its neighbours). CPU is compressible (throttling), so its
// request stays at the default. The maximum ceiling is enforced cluster-side by the
// namespace LimitRange.
func ResolveResources(in *ResourceLimits, defaultCPU, defaultMemory string) (ResolvedResources, error) {
	out := ResolvedResources{
		CPURequest:    defaultCPU,
		MemoryRequest: defaultMemory,
	}
	if in != nil {
		if in.CPU != "" {
			out.CPULimit = in.CPU
		}
		if in.Memory != "" {
			out.MemoryLimit = in.Memory
			out.MemoryRequest = in.Memory
		}
	}

	for name, q := range map[string]string{
		"cpu request":    out.CPURequest,
		"memory request": out.MemoryRequest,
		"cpu limit":      out.CPULimit,
		"memory limit":   out.MemoryLimit,
	} {
		if q == "" {
			continue
		}
		if _, err := resource.ParseQuantity(q); err != nil {
			return ResolvedResources{}, fmt.Errorf("invalid %s %q: %w", name, q, err)
		}
	}
	return out, nil
}
