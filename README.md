# mcp-k8s-ephemeral-job

[![Version](https://img.shields.io/github/v/tag/inhuman/mcp-k8s-ephemeral-job?sort=semver&style=flat-square&label=version)](https://github.com/inhuman/mcp-k8s-ephemeral-job/tags)
[![Docker Pulls](https://img.shields.io/docker/pulls/idconstruct/mcp-k8s-ephemeral-job?style=flat-square&logo=docker)](https://hub.docker.com/r/idconstruct/mcp-k8s-ephemeral-job)
[![Docker Image Version](https://img.shields.io/docker/v/idconstruct/mcp-k8s-ephemeral-job?sort=semver&style=flat-square&logo=docker&label=image)](https://hub.docker.com/r/idconstruct/mcp-k8s-ephemeral-job/tags)
[![Build](https://img.shields.io/github/actions/workflow/status/inhuman/mcp-k8s-ephemeral-job/docker-publish.yml?style=flat-square&logo=github)](https://github.com/inhuman/mcp-k8s-ephemeral-job/actions/workflows/docker-publish.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/inhuman/mcp-k8s-ephemeral-job?style=flat-square&logo=go)](https://go.dev/)
[![Go Report Card](https://goreportcard.com/badge/github.com/inhuman/mcp-k8s-ephemeral-job?style=flat-square)](https://goreportcard.com/report/github.com/inhuman/mcp-k8s-ephemeral-job)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow?style=flat-square)](LICENSE)
[![Issues](https://img.shields.io/github/issues/inhuman/mcp-k8s-ephemeral-job?style=flat-square)](https://github.com/inhuman/mcp-k8s-ephemeral-job/issues)
[![Last Commit](https://img.shields.io/github/last-commit/inhuman/mcp-k8s-ephemeral-job?style=flat-square)](https://github.com/inhuman/mcp-k8s-ephemeral-job/commits/main)

Public OSS MCP server (Go, MIT) exposing a single powerful tool — **`run_job`** — that spawns an
**ephemeral Kubernetes Job/pod** from a caller-chosen image, runs a command inside it, waits for
completion (wall-clock timeout), returns `exit_code` / `stdout` / `stderr` / **artifacts**, and
**deletes the pod**. The server **builds the pod manifest in code** — the caller passes parameters
only, never raw YAML.

It is the "operating room" counterpart to [`mcp-exec`](https://github.com/inhuman/mcp-exec) (the
"scalpel"): where `mcp-exec` runs a single Python file in a locked-down, network-less sandbox in
milliseconds, this one spins up a **full pod** with the toolchain/image you choose, **controlled
network** (clone repos, pull deps), long tasks and file **artifacts** out. They complement each
other.

Works over three transports — **stdio / HTTP / SSE** — with an identical tool set everywhere
(official [`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk)).

## The `run_job` tool

**Input**: `{ image (required), command (required), files?, env?, limits?, timeout_s?, workdir? }`
**Output**: `{ exit_code, stdout, stderr, duration_ms, status, artifacts, truncated }`

- `status` is one of `succeeded` / `failed` / `timeout` / `error`. A non-zero `exit_code` or a
  `timeout` is a **normal result**, not a tool error. Only invalid input (empty image/command,
  image not in the allowlist, bad file path) is a tool-call error.
- **Artifacts** (files from the working directory) are returned **inline** (base64) under a size cap;
  PVC / object-storage for large artifacts is Phase 2.
- The manifest is built deterministically by the server — **no raw YAML from the caller**.

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

## Run

```bash
go build -o mcp-k8s-ephemeral-job ./cmd/mcp-k8s-ephemeral-job
# Local, against a dev cluster, over stdio:
export MCP_K8S_KUBECONFIG=$HOME/.kube/config
export MCP_K8S_NAMESPACE=ephemeral-dev
export MCP_K8S_ALLOWED_IMAGES=busybox:1.36,python:3.12-slim
./mcp-k8s-ephemeral-job
```

In production the server runs **in-cluster** as a Deployment with its own ServiceAccount +
`Role`/`RoleBinding` on the ephemeral namespace + `ResourceQuota`/`LimitRange` (+ optional egress
`NetworkPolicy`). The Helm chart lives in the consumer's deploy repo; the image is public on Docker
Hub.

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
| `MCP_K8S_NAMESPACE` | namespace where ephemeral pods are spawned | `jarvis-ephemeral` |
| `MCP_K8S_DEFAULT_TIMEOUT_S` | default wall-clock timeout (s) | `60` |
| `MCP_K8S_MAX_TIMEOUT_S` | timeout ceiling (s) | `600` |
| `MCP_K8S_MAX_OUTPUT_BYTES` | combined stdout+stderr cap | `1048576` |
| `MCP_K8S_MAX_ARTIFACT_BYTES` | total artifacts size cap | `10485760` |
| `MCP_K8S_DEFAULT_CPU` | default pod CPU limit | `1` |
| `MCP_K8S_DEFAULT_MEMORY` | default pod memory limit | `512Mi` |
| `MCP_K8S_MAX_CONCURRENT` | max concurrent ephemeral pods (over → queue/error) | `10` |
| `MCP_K8S_ALLOWED_IMAGES` | strict image allowlist (CSV); **empty = deny everything** | `` |
| `MCP_K8S_SIDECAR_IMAGE` | helper sidecar image for artifact collection (pinned) | `busybox:1.36` |
| `MCP_K8S_KUBECONFIG` | path to kubeconfig; empty = in-cluster | `` |
| `MCP_K8S_AUTH_TOKEN` | if set, http/sse require `X-MCP-AUTH` header (constant-time); empty = off | `` |

## Not in v1 (MVP)

Async `submit_job → job_id` + monitor (Phase 2), secrets delivery for private clone/registry
(Phase 2), PVC / object-storage artifacts for large files, multi-cluster, proxying other MCP
servers into the pod.

## License

MIT.
