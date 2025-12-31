# Detent Job Wrapper

A lightweight marker system for tracking per-job, per-step CI data without modifying existing tooling.

## Problem

CI logs are a wall of text. Errors from different steps blend together. When parsing GitHub Actions output:

- No clear boundaries between steps in raw logs
- Exit codes lost in the noise
- Timing data requires API calls
- Tool outputs interleave unpredictably
- AI agents struggle to correlate errors to their source step

## Solution

Inject boundary markers into CI output that parsers can use to segment logs by job/step.

```
::detent::start job=build step=lint tool=eslint::
... eslint output ...
::detent::end job=build step=lint exit=1 duration=4521::
```

## Marker Format

```
::detent::{action} {key=value}...::
```

| Action | Purpose |
|--------|---------|
| `start` | Begin a tracked segment |
| `end` | Close a tracked segment with results |
| `meta` | Attach metadata without boundaries |

### Start Marker

```
::detent::start job={job} step={step} [tool={tool}]::
```

- `job` - Workflow job name (e.g., `build`, `test`, `deploy`)
- `step` - Step identifier (e.g., `lint`, `typecheck`, `unit-tests`)
- `tool` - Optional detected tool (e.g., `eslint`, `tsc`, `go`)

### End Marker

```
::detent::end job={job} step={step} exit={code} duration={ms}::
```

- `exit` - Process exit code (0 = success)
- `duration` - Milliseconds elapsed

### Meta Marker

```
::detent::meta {key=value}...::
```

Attach arbitrary metadata to current context:
- `::detent::meta commit=abc123 branch=main::`
- `::detent::meta retry=2 attempt=3::`

## Injection Methods

### Local (act runs)

The `detent` CLI wraps commands and injects markers automatically:

```bash
detent run "npm run lint"
```

Outputs:
```
::detent::start job=local step=lint tool=eslint::
... npm output ...
::detent::end job=local step=lint exit=0 duration=3200::
```

Detection happens via the existing `DetectToolFromRun()` function.

### GitHub Actions (opt-in)

A reusable action wraps steps:

```yaml
- uses: detent/wrap@v1
  with:
    run: npm run lint
    step: lint
```

Or a composite action for full job wrapping:

```yaml
jobs:
  build:
    uses: detent/job@v1
    with:
      steps: |
        - name: lint
          run: npm run lint
        - name: typecheck
          run: npm run check-types
```

### Self-hosted Runners

For complete control, a runner hook can wrap all steps automatically without workflow changes.

## Data Captured

Each segment yields:

| Field | Type | Source |
|-------|------|--------|
| `job` | string | Marker/workflow |
| `step` | string | Marker/step name |
| `tool` | string | Auto-detected |
| `exit_code` | int | Process exit |
| `duration_ms` | int | Wall clock |
| `errors` | array | Parser output |
| `error_count` | int | Parsed errors |
| `warning_count` | int | Parsed warnings |

## Parser Integration

The existing tool parser system works unchanged. Markers add segmentation:

```
[Job: build, Step: lint]
  ├── Error: src/app.ts:15:3 - 'foo' is assigned but never used
  ├── Error: src/app.ts:22:1 - Unexpected console statement
  └── Summary: 2 errors, 0 warnings, exit 1, 4.5s

[Job: build, Step: typecheck]
  ├── Error: src/types.ts:8:5 - Type 'string' not assignable to 'number'
  └── Summary: 1 error, 0 warnings, exit 1, 2.1s
```

Errors are now attributable to their exact source step.

## AI Benefits

Structured segments enable:

1. **Targeted troubleshooting** - AI sees "lint failed with 2 ESLint errors" not "build failed"
2. **Historical patterns** - Track which steps fail most, flaky tests, slow builds
3. **Smart orchestration** - Skip steps based on prior failures, parallelize intelligently
4. **Context windowing** - Feed only relevant segments to LLMs, not entire logs

## Rollout

### Phase 1: Local only

- `detent run` wraps commands with markers
- Parser respects markers for segmentation
- Zero workflow changes required

### Phase 2: GitHub Action

- Publish `detent/wrap` action
- Opt-in per step or job
- Markers visible in logs

### Phase 3: Analytics (optional)

- Persist segment data
- Trend visualization
- Failure correlation

## Non-Goals

- Replacing GitHub's native step grouping (`::group::`)
- Modifying runner infrastructure
- Storing logs (just metadata)
- Real-time streaming (batch processing is fine)

## Prior Art

- GitHub Actions `::group::` / `::endgroup::` - UI folding only, no data
- CircleCI step timing - API only, not in logs
- BuildKite annotations - Similar concept, different format
- Datadog CI Visibility - Full APM, heavy integration

## Open Questions

1. Should markers be valid GitHub workflow commands (for future native support)?
2. Should we capture stdout/stderr separately?
3. What's the maximum metadata payload before it becomes noise?
4. Should nested steps be supported (step within step)?
