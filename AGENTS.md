# Agent's Guide to `jaeger-idl`

This document summarizes key workflows, build system details, and troubleshooting tips for working with the `jaeger-idl` repository.

## Build System (`Makefile`)

The project uses a `Makefile` that heavily relies on Docker (specifically `jaegertracing/protobuf`) to ensure a consistent environment for generating code from Protobuf definitions.

### Key Targets

-   `make proto-all`: Runs all generation targets.
-   `make proto-api-v3-openapi`: Generates the OpenAPI v3 specification from `proto/api_v3`.

### OpenAPI Generation Pipeline

1.  **Generation**: Uses `protoc-gen-openapi` (Gnostic).
    -   Flags used: `fq_schema_naming=true` (avoids name collisions) and `naming=proto` (preserves snake_case).
2.  **Pruning**: Runs a custom Go tool `internal/tools/prune-openapi`.
    -   Removes unused schemas (e.g., transitively imported Gnostic types).
    -   Patches duplicate `operationId`s (e.g., `QueryService_FindTraces` -> `QueryService_FindTracesPost`).

## Working with GitHub CLI (`gh`)

To effectively gather context from Pull Requests, especially for review comments:

### Fetching Review Comments

Use the GitHub API with pagination to retrieve all comments in a raw JSON format. This is often more reliable than `gh pr view` for automated analysis.

```bash
gh api repos/:owner/:repo/pulls/:number/comments --paginate --jq '.[].body'
```

**Example:**
```bash
gh api repos/jaegertracing/jaeger-idl/pulls/185/comments --paginate --jq '.[].body'
```

### Viewing PR Details

For a quick overview of the PR description and status:

```bash
gh pr view :number
```
