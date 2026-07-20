package executor

import "context"

// Fake is an executor for unit tests ONLY (validation / manifest building / orchestration).
// Mocking the real k8s API is not allowed: Fake never talks to a cluster at all.
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
