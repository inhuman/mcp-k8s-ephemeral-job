package runjob

import (
	"sort"
	"strings"
)

// ImageResolver maps a model-supplied image reference to an exact, allowed,
// pullable image. The allowlist holds full pullable refs (e.g.
// "docker-proxy.t1.cloud/library/busybox:1.36"); callers may pass a short,
// natural name ("busybox", "busybox:latest", "docker.io/library/busybox") and
// the resolver picks the allowed ref by its base name. The pod ALWAYS runs the
// allowed ref — the caller's tag/registry is only a hint for which allowed image
// to use, so this stays a strict allowlist (no arbitrary pulls).
type ImageResolver struct {
	byBase map[string]string // base name → full allowed ref
	names  []string          // sorted base names, for the tool description
}

// NewImageResolver builds a resolver from full pullable refs.
func NewImageResolver(allowed []string) *ImageResolver {
	byBase := make(map[string]string, len(allowed))
	for _, ref := range allowed {
		byBase[baseName(ref)] = ref
	}
	names := make([]string, 0, len(byBase))
	for n := range byBase {
		names = append(names, n)
	}
	sort.Strings(names)
	return &ImageResolver{byBase: byBase, names: names}
}

// Resolve returns the allowed full ref for the requested image, matching by base
// name. ok is false when nothing matches (or the allowlist is empty).
func (r *ImageResolver) Resolve(requested string) (string, bool) {
	ref, ok := r.byBase[baseName(requested)]
	return ref, ok
}

// Names returns the friendly base names callers may use (sorted).
func (r *ImageResolver) Names() []string { return r.names }

// baseName reduces an image ref to its bare name: drop the registry/path prefix
// (everything up to the last "/") and the ":tag" or "@digest" suffix.
// "docker-proxy.t1.cloud/library/busybox:1.36" → "busybox"; "python:3.11" → "python".
func baseName(ref string) string {
	s := ref
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.IndexByte(s, '@'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(strings.TrimSpace(s))
}
