package runjob

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Validate проверяет аргументы run_job до спавна (FR-002, FR-016).
// allowed — строгий allowlist образов: пустой запрещает любой запуск.
func Validate(in Input, allowed []string, maxTimeoutS int) error {
	if strings.TrimSpace(in.Image) == "" {
		return fmt.Errorf("image is required")
	}
	if !slices.Contains(allowed, in.Image) {
		return fmt.Errorf("image %q is not allowed; use one of the allowed images exactly: %s",
			in.Image, strings.Join(allowed, ", "))
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
