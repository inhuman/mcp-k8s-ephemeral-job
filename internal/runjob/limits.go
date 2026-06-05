package runjob

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
)

// ResolvedLimits — лимиты пода, нормализованные в валидные k8s quantity-строки.
type ResolvedLimits struct {
	CPU    string
	Memory string
}

// ResolveLimits применяет дефолты и проверяет, что значения парсятся как k8s quantity.
// Потолок по максимуму навешивается LimitRange namespace на стороне кластера (FR-009).
func ResolveLimits(in *ResourceLimits, defaultCPU, defaultMemory string) (ResolvedLimits, error) {
	cpu := defaultCPU
	memory := defaultMemory
	if in != nil {
		if in.CPU != "" {
			cpu = in.CPU
		}
		if in.Memory != "" {
			memory = in.Memory
		}
	}

	if _, err := resource.ParseQuantity(cpu); err != nil {
		return ResolvedLimits{}, fmt.Errorf("invalid cpu limit %q: %w", cpu, err)
	}
	if _, err := resource.ParseQuantity(memory); err != nil {
		return ResolvedLimits{}, fmt.Errorf("invalid memory limit %q: %w", memory, err)
	}

	return ResolvedLimits{CPU: cpu, Memory: memory}, nil
}
