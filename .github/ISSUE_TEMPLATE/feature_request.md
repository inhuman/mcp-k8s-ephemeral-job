---
name: Feature request
about: Suggest a new capability, env var, or behaviour change
title: "feat: "
labels: enhancement
---

## Problem

<!-- What you're trying to do that mcp-k8s-ephemeral-job can't do today, or does awkwardly. Concrete scenario, not abstract wishlist. -->

## Proposed solution

<!-- What you'd like it to do. New env var? New tool field? Different default? Async submit_job? -->

## Alternatives considered

<!-- Workarounds you've tried, or simpler approaches you ruled out and why. -->

## Constraints / non-goals

<!-- What this feature should NOT do. Note: the security invariants (ephemeral, namespace-scoped,
     least privilege, resource ceilings, GC, caller-data privacy) are non-negotiable — see the constitution. -->

## Example usage

<!-- A snippet showing how the feature would be used, even if pseudo. -->

```json
{ "image": "...", "command": ["..."], "your_new_field": "..." }
```

```sh
MCP_K8S_YOUR_NEW_VAR=value ./mcp-k8s-ephemeral-job
```

## Impact on existing users

<!-- Backward compatibility. Does this change a default? Touch the run_job tool contract (input/output schema)? -->
