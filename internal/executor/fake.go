package executor

import "context"

// Fake — исполнитель ТОЛЬКО для unit-тестов (валидация/сборка манифеста/оркестрация).
// Моки реального k8s API запрещены (Принцип V): Fake не обращается к кластеру вовсе.
type Fake struct {
	LastSpec Spec
	Result   Result
	Err      error
}

func (f *Fake) Run(_ context.Context, spec Spec) (Result, error) {
	f.LastSpec = spec
	if f.Err != nil {
		return Result{}, f.Err
	}
	return f.Result, nil
}
