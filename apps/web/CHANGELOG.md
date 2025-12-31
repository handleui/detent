# @detent/web

## 0.1.0

### Minor Changes

- cec0217: Split tool parsing into dedicated per-language parsers, improve check UI, and add Sentry monitoring

  # Tool Parsing Improvements

  - Split monolithic error parser into dedicated per-tool parsers (Go, TypeScript, ESLint, Rust)
  - Add ESLint parser supporting stylish, compact, and unix output formats
  - Add Rust/Cargo parser with Clippy lint support and multi-line error handling
  - Implement parser registry with priority-based routing and confidence scoring
  - Remove premature tool patterns for unimplemented parsers (Python, Java, Ruby, etc.)

  # Check Command UI

  - Improve error display with structured output and better formatting
  - Add tool detection feedback showing which parsers are being used
  - Better progress indicators and status messages

  # Sentry Integration

  - Add crash reporting and error tracking via Sentry SDK
  - Track unsupported tool usage to prioritize parser development
  - PII scrubbing and filtering for sensitive data

  # Config & Schema

  - Add config migrations for schema version upgrades
  - Update JSON schema with new configuration options
  - Improve validation and error messages for invalid configs

  # Frankenstein Command

  - Add experimental `frankenstein` command for parallel Claude iterations
  - Support for testing AI-powered error fixing workflows
