---
"@detent/parser": minor
---

Initial release of the Detent parser package

### Features

- **Multi-Language Error Parsing**: Extract structured errors from TypeScript, ESLint, Go, Python, Rust, and generic output
- **GitHub Actions Context Parser**: Parse GitHub Actions logs with timestamp stripping and workflow command extraction
- **Act Context Parser**: Parse local Act runner output with ANSI escape handling
- **CI Event System**: Typed event stream for job start/end, step execution, and error detection
- **Error Registry**: Central registry for discovered errors with deduplication

### Parsers Included

- **TypeScript**: TSC errors with file, line, column, and error code extraction
- **ESLint**: Lint violations with rule IDs and fix suggestions
- **Go**: Build and test errors from `go build`, `go test`
- **Python**: Syntax errors, tracebacks, and pytest failures
- **Rust**: Cargo build errors with span information
- **Infrastructure**: Generic command failures and exit codes

### Technical Details

- Event-driven architecture for streaming log processing
- Severity levels: error, warning, info
- Code snippet extraction with context lines
- Serialization support for persistence
- Comprehensive test suite with real-world log samples
