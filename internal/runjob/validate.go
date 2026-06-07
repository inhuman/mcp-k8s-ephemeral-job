package runjob

import (
	"fmt"
	"regexp"
	"strings"
)

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Validate проверяет аргументы run_job до спавна (FR-002, FR-016).
// resolver maps the requested image to an allowed pullable ref; an empty
// allowlist resolves nothing, so every call is rejected.
func Validate(in Input, resolver *ImageResolver, maxTimeoutS int) error {
	if strings.TrimSpace(in.Image) == "" {
		return fmt.Errorf("image is required")
	}
	if _, ok := resolver.Resolve(in.Image); !ok {
		return fmt.Errorf("image %q is not available; use one of: %s",
			in.Image, strings.Join(resolver.Names(), ", "))
	}
	if len(in.Command) == 0 {
		return fmt.Errorf("command is required")
	}

	for i, f := range in.Files {
		if err := validateFilePath(f.Path); err != nil {
			return fmt.Errorf("files[%d]: %w", i, err)
		}
		if f.ContentB64 == "" {
			return fmt.Errorf("files[%d]: content_b64 is required", i)
		}
	}

	for k := range in.Env {
		if !envKeyRe.MatchString(k) {
			return fmt.Errorf("invalid env key %q", k)
		}
	}

	if in.TimeoutS < 0 {
		return fmt.Errorf("timeout_s must be non-negative")
	}
	if in.TimeoutS > maxTimeoutS {
		return fmt.Errorf("timeout_s %d exceeds max %d", in.TimeoutS, maxTimeoutS)
	}

	if in.Clone != nil {
		if err := validateClone(in.Clone); err != nil {
			return err
		}
	}

	return nil
}

// shellMeta matches characters that could break out of a quoted shell value in
// the clone script; repo_url/ref/subdir are caller-supplied and must be inert.
var shellMeta = regexp.MustCompile("[`$;&|<>()\\\\\"'\n\r\t ]")

func validateClone(c *CloneInput) error {
	if !strings.HasPrefix(c.RepoURL, "https://") {
		return fmt.Errorf("clone.repo_url must start with https://")
	}
	if strings.ContainsRune(c.RepoURL, '@') {
		return fmt.Errorf("clone.repo_url must NOT contain credentials (no '@') — the server injects them")
	}
	if c.Ref == "" {
		return fmt.Errorf("clone.ref is required")
	}
	// ".." is forbidden everywhere: the host derived from repo_url is used as a
	// file path under the creds mount (/git-creds/<host>), so traversal must not slip in.
	for name, v := range map[string]string{"repo_url": c.RepoURL, "ref": c.Ref, "subdir": c.Subdir} {
		if strings.Contains(v, "..") {
			return fmt.Errorf("clone.%s must not contain ..", name)
		}
		// repo_url legitimately has no shell metas except the scheme's "//"; ref/subdir must be plain.
		if name == "repo_url" {
			if shellMeta.MatchString(strings.TrimPrefix(v, "https://")) {
				return fmt.Errorf("clone.repo_url contains illegal characters")
			}
			continue
		}
		if v != "" && shellMeta.MatchString(v) {
			return fmt.Errorf("clone.%s contains illegal shell characters", name)
		}
	}
	return nil
}

func validateFilePath(p string) error {
	if p == "" {
		return fmt.Errorf("path is required")
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("path %q must be relative", p)
	}
	for elem := range strings.SplitSeq(p, "/") {
		if elem == ".." {
			return fmt.Errorf("path %q must not contain ..", p)
		}
	}
	return nil
}
