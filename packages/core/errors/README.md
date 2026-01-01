# Stack Trace Extraction

## What It Does

Captures multi-line stack traces from CI output. Doesn't parse structure - just groups continuation lines with the error.

## Supported Formats

### Python (pytest, unittest, etc.)

```bash
Traceback (most recent call last):
  File "test.py", line 10, in test_foo
    assert False
AssertionError: test failed
```

### JavaScript/TypeScript (vitest, jest, mocha)

```bash
  at Object.<anonymous> (file.test.ts:15:10)
  at Promise.then.completed (node_modules/jest/index.js:123:5)
  at new Promise (<anonymous>)
```

### Go (go test)

```bash
panic: runtime error
goroutine 1 [running]:
main.foo(...)
    /path/file.go:42
```

### Test Failures (any framework)

```bash
--- FAIL: TestName (0.00s)
    file_test.go:10: expected true, got false
        detailed output here
```

## How It Works

1. Detects start pattern (e.g., `Traceback`, `panic:`, `--- FAIL:`)
2. Accumulates indented/continuation lines
3. Stops when pattern breaks (non-continuation line)
4. Stores full trace in `ExtractedError.StackTrace` field

## Implementation

- **Patterns**: `internal/errors/patterns.go` - regex for detection
- **Extraction**: `internal/errors/extractor.go` - accumulation logic
- **Storage**: `internal/persistence/sqlite.go` - `errors.stack_trace` column
