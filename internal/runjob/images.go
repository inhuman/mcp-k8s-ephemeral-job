package runjob

import (
	"sort"
	"strings"
)

// ImageResolver maps a model-supplied image reference to an exact, allowed,
// pullable image. The allowlist holds full pullable refs (e.g.
// "registry.example.com/library/busybox:1.36"); callers may pass a short,
// natural name ("busybox", "busybox:latest", "docker.io/library/busybox") and
// the resolver picks the allowed ref by its base name. The pod ALWAYS runs the
// allowed ref — the caller's tag/registry is only a hint for which allowed image
// to use, so this stays a strict allowlist (no arbitrary pulls).
type ImageResolver struct {
	byTag  map[string]string // "base:tag" → full allowed ref (exact version)
	byBase map[string]string // base name → default full ref (first listed per base)
	names  []string          // sorted display names (base, or "base:tag" when a base has multiple versions)
}

// NewImageResolver builds a resolver from full pullable refs. When a base name
// has several versions, the FIRST listed becomes its default and every version
// is reachable by its exact "base:tag".
func NewImageResolver(allowed []string) *ImageResolver {
	byTag := make(map[string]string, len(allowed))
	byBase := make(map[string]string, len(allowed))
	baseCount := make(map[string]int)
	for _, ref := range allowed {
		b := baseName(ref)
		if bt := baseTag(ref); bt != "" {
			byTag[bt] = ref
		}
		if _, seen := byBase[b]; !seen {
			byBase[b] = ref // first listed per base wins as the default
		}
		baseCount[b]++
	}
	// Display names: bare base when a base has one version, else "base:tag" per version.
	names := make([]string, 0, len(allowed))
	if len(byTag) > 0 || len(byBase) > 0 {
		seen := make(map[string]struct{})
		for _, ref := range allowed {
			b := baseName(ref)
			name := b
			if baseCount[b] > 1 {
				name = baseTag(ref)
			}
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return &ImageResolver{byTag: byTag, byBase: byBase, names: names}
}

// Resolve returns the allowed full ref for the requested image. An exact
// "base:tag" match (a specific version) wins; otherwise the base-name default is
// used (tag ignored). ok is false when nothing matches or the allowlist is empty.
func (r *ImageResolver) Resolve(requested string) (string, bool) {
	if ref, ok := r.byTag[baseTag(requested)]; ok {
		return ref, true
	}
	ref, ok := r.byBase[baseName(requested)]
	return ref, ok
}

// Names returns the names callers may use (sorted): a bare base name, or
// "base:tag" for bases that have more than one allowed version.
func (r *ImageResolver) Names() []string { return r.names }

// baseName reduces an image ref to its bare name: drop the registry/path prefix
// (everything up to the last "/") and the ":tag" or "@digest" suffix.
// "registry.example.com/library/busybox:1.36" → "busybox"; "python:3.11" → "python".
func baseName(ref string) string {
	s := stripRegistry(ref)
	if i := strings.IndexByte(s, '@'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// baseTag reduces a ref to "name:tag" without the registry/path prefix, lowercased.
// "registry.example.com/library/busybox:1.36" → "busybox:1.36". Returns "" when
// there is no tag (a bare "busybox" has no specific version to key on).
func baseTag(ref string) string {
	s := strings.ToLower(strings.TrimSpace(stripRegistry(ref)))
	if i := strings.IndexByte(s, '@'); i >= 0 { // digests aren't version tags here
		return ""
	}
	if strings.IndexByte(s, ':') < 0 {
		return ""
	}
	return s
}

func stripRegistry(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}
