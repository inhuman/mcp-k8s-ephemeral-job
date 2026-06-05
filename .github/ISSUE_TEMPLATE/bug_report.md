---
name: Bug report
about: Something doesn't work as documented
title: "bug: "
labels: bug
---

## Summary

<!-- One sentence: what's broken? -->

## Environment

- mcp-k8s-ephemeral-job version: <!-- image tag (e.g. idconstruct/mcp-k8s-ephemeral-job:v0.1.0) or "main@abc1234" -->
- Transport: <!-- stdio / http / sse -->
- How you run it: <!-- in-cluster Deployment / built locally with KUBECONFIG -->
- Kubernetes flavor/version: <!-- e.g. OpenShift 4.15 (k8s 1.28), kind, k3s -->
- Relevant env vars: <!-- e.g. MCP_K8S_NAMESPACE, MCP_K8S_ALLOWED_IMAGES, MCP_K8S_DEFAULT_TIMEOUT_S, MCP_K8S_AUTH_TOKEN=set/unset -->

## Reproduction

The `run_job` tool input that reproduces the issue (strip secrets):

```json
{ "image": "busybox:1.36", "command": ["sh", "-c", "echo hi"], "timeout_s": 30 }
```

How the server was launched:

```sh
MCP_K8S_NAMESPACE=ephemeral-dev MCP_K8S_ALLOWED_IMAGES=busybox:1.36 ./mcp-k8s-ephemeral-job
```

## Expected behaviour

<!-- What you thought should happen. -->

## Actual behaviour

<!-- What happened instead. Paste the relevant fields of the tool result and/or short log lines.
     Do NOT paste full stdout/stderr/artifacts if large or sensitive. -->

```
<paste run_job result / short log output here>
```

## Cluster state (if relevant)

<!-- e.g. `kubectl get jobs,pods -n <ns>` output, orphaned pods, quota/LimitRange hits -->

## Additional context

<!-- Anything else relevant: limits hit (truncated/timeout), images, RBAC errors, etc. -->
