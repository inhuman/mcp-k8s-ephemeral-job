## What

<!-- One paragraph: what does this PR change? -->

## Why

<!-- The motivation. Link to the issue if one exists. -->

Closes #

## How

<!-- Brief implementation notes. What did you change architecturally? Any tricky bits the reviewer should pay attention to? -->

## Testing

- [ ] `go vet ./...` clean
- [ ] `go test ./... -count=1` all green
- [ ] `go test -short ./...` passes without a cluster
- [ ] New tests added for new behaviour (unit / integration)
- [ ] If spawn/collect/teardown behaviour is affected: `envtest` (contract) and/or `kind` (e2e) pass
- [ ] If image build is affected: `docker build .` succeeds locally

## Security model

<!-- The invariants are non-negotiable (constitution). Confirm none are weakened: -->

- [ ] Spawned pods stay ephemeral and are deleted (success / failure / timeout)
- [ ] Server RBAC stays namespace-scoped (Role, not ClusterRole); pods non-root / cap-drop=ALL
- [ ] Resource ceilings hold (LimitRange + ResourceQuota + wall-clock timeout → kill → GC)
- [ ] Image allowlist enforced before spawn; no raw YAML accepted from the caller
- [ ] Caller data (`command` / `files` / output / artifacts) is never persisted or fully logged

## Backward compatibility

<!-- Does this change a default, an env var, or the run_job tool input/output schema? If yes — explain the migration path. Schema breaks require a MAJOR bump. -->

## Checklist

- [ ] Commit message follows Conventional Commits (`feat(scope):`, `fix:`, `docs:`, etc.)
- [ ] Godoc added/updated for new exported identifiers
- [ ] README / CLAUDE.md updated if user-visible behaviour changed
- [ ] No new core dependencies (or: amendment to Constitution included)
- [ ] No secrets / tokens in the diff
