# mcp-k8s-ephemeral-job

**English** | [Русский](README.ru.md)

[![Version](https://img.shields.io/github/v/tag/inhuman/mcp-k8s-ephemeral-job?sort=semver&style=flat-square&label=version)](https://github.com/inhuman/mcp-k8s-ephemeral-job/tags)
[![Docker Pulls](https://img.shields.io/docker/pulls/idconstruct/mcp-k8s-ephemeral-job?style=flat-square&logo=docker)](https://hub.docker.com/r/idconstruct/mcp-k8s-ephemeral-job)
[![Docker Image Version](https://img.shields.io/docker/v/idconstruct/mcp-k8s-ephemeral-job?sort=semver&style=flat-square&logo=docker&label=image)](https://hub.docker.com/r/idconstruct/mcp-k8s-ephemeral-job/tags)
[![Build](https://img.shields.io/github/actions/workflow/status/inhuman/mcp-k8s-ephemeral-job/docker-publish.yml?style=flat-square&logo=github)](https://github.com/inhuman/mcp-k8s-ephemeral-job/actions/workflows/docker-publish.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/inhuman/mcp-k8s-ephemeral-job?style=flat-square&logo=go)](https://go.dev/)
[![Go Report Card](https://goreportcard.com/badge/github.com/inhuman/mcp-k8s-ephemeral-job?style=flat-square)](https://goreportcard.com/report/github.com/inhuman/mcp-k8s-ephemeral-job)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow?style=flat-square)](LICENSE)
[![Issues](https://img.shields.io/github/issues/inhuman/mcp-k8s-ephemeral-job?style=flat-square)](https://github.com/inhuman/mcp-k8s-ephemeral-job/issues)
[![Last Commit](https://img.shields.io/github/last-commit/inhuman/mcp-k8s-ephemeral-job?style=flat-square)](https://github.com/inhuman/mcp-k8s-ephemeral-job/commits/main)

Public OSS MCP server (Go, MIT) that spawns an **ephemeral Kubernetes Job/pod** from a
caller-chosen image, runs a command inside it, returns `exit_code` / `stdout` / **artifacts**, and
**deletes the pod**. The server **builds the pod manifest in code** — the caller passes parameters
only, never raw YAML.

It is the "operating room" counterpart to [`mcp-exec`](https://github.com/inhuman/mcp-exec) (the
"scalpel"): where `mcp-exec` runs a single Python file in a locked-down, network-less sandbox in
milliseconds, this one spins up a **full pod** with the toolchain/image you choose, **controlled
network** (clone repos, pull deps), long tasks and file **artifacts** out. They complement each
other.

Works over three transports — **stdio / HTTP / SSE** — with an identical tool set everywhere
(official [`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk)).

## Tools

### `run_job` — run and wait

**Input**: `{ image (required), command (required), files?, env?, limits?, timeout_s?, workdir?, clone? }`
**Output**: `{ exit_code, stdout, stderr, duration_ms, status, artifacts, truncated }`

- `status` is one of `succeeded` / `failed` / `timeout` / `error`. A non-zero `exit_code` or a
  `timeout` is a **normal result**, not a tool error. Only invalid input (empty image/command,
  image not in the allowlist, bad file path) is a tool-call error.
- `stdout` carries the container's **combined** stdout+stderr — Kubernetes merges the two streams in
  pod logs. `stderr` is reserved and always empty, so adding stream separation later stays
  backward-compatible.
- **Artifacts** (files from the working directory) come back **inline** (base64) under a size cap.
  Exceeding a cap never loses data silently: the matching `truncated` flag is set.
- The manifest is built deterministically by the server — **no raw YAML from the caller**.

### `submit_job` / `fetch_job` — run in the background

`submit_job` takes the same arguments as `run_job` but returns a `job_token` **immediately**;
`fetch_job` collects the result later. This is what lets an agent start a long job (a full test
battery, a build) and keep working instead of idling inside one synchronous call for its whole
wall-clock time.

- `fetch_job` returns `status=running` while the job is in flight; pass `wait_s` (≤120) to
  long-poll instead of hammering. It answers as soon as the job is done rather than sitting out the
  full wait.
- Results are retained for 60 minutes and can be fetched more than once.
- Arguments are validated at **submit** time, so a bad image fails the submit while the caller can
  still fix it.
- Handles live in memory (the server is single-replica by design): a restart drops pending tokens
  and the caller simply resubmits.

### Cloning a repository

With `clone: { repo_url, ref, subdir? }` an init container checks the repo out into the working
directory before the command runs. **The caller never handles credentials**: the server holds a
secret with one token per git host, mounts it **only** on the cloner, and the token is masked in
`.git/config` afterwards — the main container never sees it. The `clone` field is accepted only when
the operator has configured `MCP_K8S_CLONE_IMAGE` + `MCP_K8S_CLONE_SECRET`.

## Security model (invariants)

Per run: a **fresh ephemeral pod**, deleted afterwards (success / failure / timeout). The server's
RBAC is **namespace-scoped** (`Role`/`RoleBinding`, never `ClusterRole`) — `create/delete jobs,pods`
plus `pods/log`, `pods/exec` in **one** namespace. Spawned pods run with `cap-drop=ALL`,
`no-privilege-escalation`, seccomp `RuntimeDefault`. The blast radius is that one namespace:
`LimitRange` (per-pod default+max) + `ResourceQuota` (namespace ceiling) + wall-clock timeout
(→ kill) + TTL/owner-reference GC + a concurrency cap. Images must pass a **strict allowlist**
(`MCP_K8S_ALLOWED_IMAGES`; empty = nothing runs). Caller data (`command` / `files` / output /
artifacts) is never persisted and never logged in full — only metadata.

> `run_job` is the most powerful surface there is (it creates pods). When embedding it in an agent,
> gate it behind that agent's tool-policy (trusted roles only).

**Network note:** the pod's network is **not** disabled (it's needed to clone repos / pull deps).
Egress is controlled by a namespace `NetworkPolicy` (allowlist) as a deployment concern, not a
right baked into the code. The invariant is ephemerality + deletion, not the absence of network.

**Resources:** the server's `MCP_K8S_DEFAULT_CPU`/`MEMORY` are pod **requests** (the scheduler's
reservation). Limits are set only when the caller passes `limits`; otherwise the ceiling comes from
the namespace `LimitRange`. Passing `limits.memory` also raises the memory request to match, since
memory is incompressible and the pod must land on a node that actually has it.

## Run

Published in the [MCP Registry](https://registry.modelcontextprotocol.io) as
`io.github.inhuman/mcp-k8s-ephemeral-job`; the image is on Docker Hub as
[`idconstruct/mcp-k8s-ephemeral-job`](https://hub.docker.com/r/idconstruct/mcp-k8s-ephemeral-job).

```bash
docker run --rm -i \
  -v "$HOME/.kube:/kube:ro" \
  -e MCP_K8S_KUBECONFIG=/kube/config \
  -e MCP_K8S_NAMESPACE=ephemeral-dev \
  -e MCP_K8S_ALLOWED_IMAGES=busybox:1.36,python:3.12-slim \
  idconstruct/mcp-k8s-ephemeral-job:latest
```

From source, against a dev cluster, over stdio:

```bash
go build -o mcp-k8s-ephemeral-job ./cmd/mcp-k8s-ephemeral-job
export MCP_K8S_KUBECONFIG=$HOME/.kube/config
export MCP_K8S_NAMESPACE=ephemeral-dev
export MCP_K8S_ALLOWED_IMAGES=busybox:1.36,python:3.12-slim
./mcp-k8s-ephemeral-job
```

In production the server runs **in-cluster** as a Deployment with its own ServiceAccount +
`Role`/`RoleBinding` on the ephemeral namespace + `ResourceQuota`/`LimitRange` (+ optional egress
`NetworkPolicy`), usually on the `http` transport.

### Example call (`run_job`)

```json
{
  "image": "python:3.12-slim",
  "command": ["python", "gen.py"],
  "files": [{ "path": "gen.py", "content_b64": "<base64 of a script writing out.png>" }],
  "limits": { "cpu": "500m", "memory": "256Mi" },
  "timeout_s": 30
}
```

Returns `exit_code`, captured output, and `out.png` inline in `artifacts`. Afterwards the pod is
gone (`kubectl get jobs,pods -n $NS` is empty).

### Optional auth (HTTP/SSE)

Set `MCP_K8S_AUTH_TOKEN` to require every HTTP/SSE request to carry a matching `X-MCP-AUTH` header
(constant-time compare; `401` otherwise). Empty token disables it. Not applicable to stdio.

## Configuration

| Env var | Purpose | Default |
|---|---|---|
| `MCP_K8S_TRANSPORT` | `stdio` \| `http` \| `sse` | `stdio` |
| `MCP_K8S_ADDR` | listen address for http/sse | `:8080` |
| `MCP_K8S_NAMESPACE` | namespace where ephemeral pods are spawned | `mcp-ephemeral` |
| `MCP_K8S_DEFAULT_TIMEOUT_S` | default wall-clock timeout (s) | `60` |
| `MCP_K8S_MAX_TIMEOUT_S` | timeout ceiling (s) | `600` |
| `MCP_K8S_MAX_OUTPUT_BYTES` | combined stdout+stderr cap | `1048576` |
| `MCP_K8S_MAX_ARTIFACT_BYTES` | total artifacts size cap | `10485760` |
| `MCP_K8S_DEFAULT_CPU` | pod CPU request (scheduling reservation; limits come from the caller's `limits` or the namespace LimitRange) | `1` |
| `MCP_K8S_DEFAULT_MEMORY` | pod memory request (see above) | `512Mi` |
| `MCP_K8S_MAX_CONCURRENT` | max concurrent ephemeral pods (over → queue/error) | `10` |
| `MCP_K8S_ALLOWED_IMAGES` | strict image allowlist (CSV); **empty = deny everything** | `` |
| `MCP_K8S_SIDECAR_IMAGE` | helper sidecar image for artifact collection (pinned) | `busybox:1.36` |
| `MCP_K8S_CLONE_IMAGE` | image with git for the clone init container; empty = `clone` unavailable | `` |
| `MCP_K8S_CLONE_SECRET` | secret holding one token per git host (key = host); mounted only on the cloner | `` |
| `MCP_K8S_CACHE_PVC` | existing PVC mounted into every job pod as a shared cache; empty = no cache | `` |
| `MCP_K8S_CACHE_MOUNT_PATH` | where the cache PVC is mounted, e.g. `/go/pkg/mod` | `` |
| `MCP_K8S_JOB_EXTRA_ENV` | JSON object `{"KEY":"value"}` added to every job pod; caller keys win | `` |
| `MCP_K8S_KUBECONFIG` | path to kubeconfig; empty = in-cluster | `` |
| `MCP_K8S_AUTH_TOKEN` | if set, http/sse require `X-MCP-AUTH` header (constant-time); empty = off | `` |

Both cache variables must be set together for the cache to mount. The PVC itself is provisioned
out-of-band (helm/manifest); the server only references it by name and fails fast at startup if it
is missing.

## Not implemented

PVC / object-storage delivery for artifacts too large to inline, multi-cluster support, and proxying
other MCP servers into the pod.

## License

MIT.
