/**
 * PII and sensitive data redaction utilities.
 * Migrated from packages/core/extract/extractor.go
 */

import type { ErrorReport, ExtractedError } from "./types.js";

// ============================================================================
// Constants
// ============================================================================

const maxPatternLineLength = 500;

// ============================================================================
// Sensitive Pattern Types
// ============================================================================

/**
 * Defines a pattern for detecting and redacting sensitive data.
 *
 * @example
 * ```typescript
 * const pattern: RedactionPattern = {
 *   name: "aws_access_key",
 *   pattern: /AKIA[0-9A-Z]{16}/g,
 *   replacement: "[AWS_ACCESS_KEY]",
 * };
 * ```
 */
export interface RedactionPattern {
  /** Human-readable identifier for the pattern (e.g., "aws_access_key", "email") */
  readonly name: string;
  /** RegExp pattern to match sensitive data. Must use global flag (g). */
  readonly pattern: RegExp;
  /** Replacement string. Can use capture groups like $1. */
  readonly replacement: string;
}

/**
 * @deprecated Use RedactionPattern instead
 */
interface Sanitizer {
  readonly pattern: RegExp;
  readonly replacement: string;
}

// ============================================================================
// Sensitive Patterns
// ============================================================================

/**
 * Comprehensive regex patterns for detecting and redacting sensitive data.
 *
 * SECURITY: These patterns prevent leaking credentials, tokens, paths, and PII.
 * Pattern order matters: More specific patterns should come before generic ones
 * to prevent partial matches from breaking the more specific patterns.
 *
 * Each pattern is compiled once at module load time for performance.
 */
export const redactionPatterns: readonly RedactionPattern[] = [
  // =========================================================================
  // Platform-specific tokens (most specific, check first)
  // =========================================================================

  /**
   * OpenAI API keys
   * @example "sk-proj-abc123def456ghi789jkl012mno345pqr678"
   */
  {
    name: "openai_key",
    pattern: /sk-[A-Za-z0-9]{32,}/g,
    replacement: "[OPENAI_KEY]",
  },

  /**
   * Anthropic API keys
   * @example "sk-ant-api03-abcdefghij1234567890"
   */
  {
    name: "anthropic_key",
    pattern: /sk-ant-api\d+-[A-Za-z0-9_-]{10,}/g,
    replacement: "[ANTHROPIC_KEY]",
  },

  /**
   * Stripe live secret keys
   * @example "sk_live_EXAMPLE_KEY_DO_NOT_USE"
   */
  {
    name: "stripe_live_secret",
    pattern: /sk_live_[A-Za-z0-9]{24,}/g,
    replacement: "[STRIPE_LIVE_KEY]",
  },

  /**
   * Stripe test secret keys
   * @example "sk_test_51ABC123def456GHI789jkl012"
   */
  {
    name: "stripe_test_secret",
    pattern: /sk_test_[A-Za-z0-9]{24,}/g,
    replacement: "[STRIPE_TEST_KEY]",
  },

  /**
   * Stripe live publishable keys
   * @example "pk_live_51ABC123def456GHI789jkl012"
   */
  {
    name: "stripe_live_pubkey",
    pattern: /pk_live_[A-Za-z0-9]{24,}/g,
    replacement: "[STRIPE_LIVE_PUBKEY]",
  },

  /**
   * Stripe test publishable keys
   * @example "pk_test_51ABC123def456GHI789jkl012"
   */
  {
    name: "stripe_test_pubkey",
    pattern: /pk_test_[A-Za-z0-9]{24,}/g,
    replacement: "[STRIPE_TEST_PUBKEY]",
  },

  /**
   * GitHub personal access tokens (classic)
   * @example "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
   */
  {
    name: "github_pat_classic",
    pattern: /ghp_[A-Za-z0-9]{36,}/g,
    replacement: "[GITHUB_TOKEN]",
  },

  /**
   * GitHub OAuth access tokens
   * @example "gho_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
   */
  {
    name: "github_oauth",
    pattern: /gho_[A-Za-z0-9]{36,}/g,
    replacement: "[GITHUB_OAUTH_TOKEN]",
  },

  /**
   * GitHub user-to-server tokens
   * @example "ghu_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
   */
  {
    name: "github_user_token",
    pattern: /ghu_[A-Za-z0-9]{36,}/g,
    replacement: "[GITHUB_USER_TOKEN]",
  },

  /**
   * GitHub server-to-server tokens
   * @example "ghs_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
   */
  {
    name: "github_server_token",
    pattern: /ghs_[A-Za-z0-9]{36,}/g,
    replacement: "[GITHUB_SERVER_TOKEN]",
  },

  /**
   * GitHub refresh tokens
   * @example "ghr_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
   */
  {
    name: "github_refresh_token",
    pattern: /ghr_[A-Za-z0-9]{36,}/g,
    replacement: "[GITHUB_REFRESH_TOKEN]",
  },

  /**
   * GitHub fine-grained personal access tokens
   * @example "github_pat_11ABCDEFGH0123456789_abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
   */
  {
    name: "github_pat_fine_grained",
    pattern: /github_pat_[A-Za-z0-9]{22}_[A-Za-z0-9]{59}/g,
    replacement: "[GITHUB_PAT]",
  },

  /**
   * GitHub fine-grained PAT (shorter variant)
   * @example "github_pat_xxxxxxxxxxxxxxxxxxxxxx"
   */
  {
    name: "github_pat_short",
    pattern: /github_pat_[A-Za-z0-9_]{22,}/g,
    replacement: "[GITHUB_PAT]",
  },

  /**
   * GitLab personal access tokens
   * @example "glpat-xxxxxxxxxxxxxxxxxxxx"
   */
  {
    name: "gitlab_pat",
    pattern: /glpat-[A-Za-z0-9-]{20,}/g,
    replacement: "[GITLAB_PAT]",
  },

  /**
   * AWS access key IDs (permanent)
   * @example "AKIAIOSFODNN7EXAMPLE"
   */
  {
    name: "aws_access_key",
    pattern: /AKIA[0-9A-Z]{16}/g,
    replacement: "[AWS_ACCESS_KEY]",
  },

  /**
   * AWS temporary access key IDs (STS)
   * @example "ASIAIOSFODNN7EXAMPLE"
   */
  {
    name: "aws_temp_key",
    pattern: /ASIA[0-9A-Z]{16}/g,
    replacement: "[AWS_TEMP_KEY]",
  },

  /**
   * AWS secret access keys (in key=value format)
   * @example "aws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
   */
  {
    name: "aws_secret_key_kv",
    pattern:
      /aws[_-]?secret[_-]?access[_-]?key\s*[:=]\s*['"]?[A-Za-z0-9/+=]{40}['"]?/gi,
    replacement: "aws_secret_access_key=[REDACTED]",
  },

  /**
   * AWS secret access keys (standalone 40-char base64)
   * @example "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
   */
  {
    name: "aws_secret_key_standalone",
    pattern: /\b[A-Za-z0-9/+=]{40}\b/g,
    replacement: "[AWS_SECRET_KEY]",
  },

  /**
   * NPM access tokens
   * @example "npm_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
   */
  {
    name: "npm_token",
    pattern: /npm_[A-Za-z0-9]{36}/g,
    replacement: "[NPM_TOKEN]",
  },

  /**
   * Discord bot/user tokens
   * @example "MTAxNjQ5.GxY3Kw.abc123-xyz789_DEF456ghi012JKL"
   */
  {
    name: "discord_token",
    pattern: /[MN][A-Za-z\d]{23,}\.[\w-]{6}\.[\w-]{27}/g,
    replacement: "[DISCORD_TOKEN]",
  },

  /**
   * Slack bot/app/user tokens
   * @example "xoxb-EXAMPLE-TOKEN-DO-NOT-USE"
   */
  {
    name: "slack_token",
    pattern: /xox[baprs]-[A-Za-z0-9-]+/g,
    replacement: "[SLACK_TOKEN]",
  },

  /**
   * Twilio API keys
   * @example "SKxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
   */
  {
    name: "twilio_key",
    pattern: /SK[a-f0-9]{32}/g,
    replacement: "[TWILIO_KEY]",
  },

  /**
   * SendGrid API keys
   * @example "SG.xxxxxxxxxxxxxxxxxxxx.xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
   */
  {
    name: "sendgrid_key",
    pattern: /SG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}/g,
    replacement: "[SENDGRID_KEY]",
  },

  /**
   * Google API keys
   * @example "AIzaSyDaGmWKa4JsXZ-HjGw7ISLn_3namBGewQe"
   */
  {
    name: "google_api_key",
    pattern: /AIza[0-9A-Za-z_-]{35}/g,
    replacement: "[GOOGLE_API_KEY]",
  },

  /**
   * Firebase API keys (same format as Google)
   * @example "AIzaSyDaGmWKa4JsXZ-HjGw7ISLn_3namBGewQe"
   */
  {
    name: "firebase_key",
    pattern: /AIza[0-9A-Za-z_-]{35}/g,
    replacement: "[FIREBASE_KEY]",
  },

  /**
   * Mailchimp API keys
   * @example "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx-us21"
   */
  {
    name: "mailchimp_key",
    pattern: /[a-f0-9]{32}-us\d{1,2}/g,
    replacement: "[MAILCHIMP_KEY]",
  },

  /**
   * UUIDs (often used as API keys or session tokens)
   * @example "550e8400-e29b-41d4-a716-446655440000"
   */
  {
    name: "uuid",
    pattern: /[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}/gi,
    replacement: "[UUID]",
  },

  // =========================================================================
  // Generic key/value patterns (after specific tokens)
  // =========================================================================

  /**
   * API keys and secrets in key=value format
   * @example "api_key=abc123def456", "secret_key: mysecretvalue"
   */
  {
    name: "api_key_kv",
    pattern:
      /(api[_-]?key|apikey|api[_-]?secret|secret[_-]?key)\s*[:=]\s*['"]?[A-Za-z0-9_-]{8,}['"]?/gi,
    replacement: "$1=[REDACTED]",
  },

  /**
   * Generic auth tokens in key=value format
   * @example "password=mypassword123", "token: bearer_xyz"
   */
  {
    name: "auth_token_kv",
    pattern:
      /(token|bearer|auth|password|passwd|pwd|secret)\s*[:=]\s*['"]?[A-Za-z0-9_.-]{8,}['"]?/gi,
    replacement: "$1=[REDACTED]",
  },

  /**
   * Environment variable style secrets
   * @example "DB_PASSWORD=supersecret", "AWS_SECRET_KEY=abc123"
   */
  {
    name: "env_var_secret",
    pattern:
      /([A-Z_]+(?:SECRET|PASSWORD|TOKEN|KEY|CREDENTIAL|AUTH)[A-Z_]*)\s*[:=]\s*['"]?[^\s'"]{8,}['"]?/g,
    replacement: "$1=[REDACTED]",
  },

  // =========================================================================
  // JWT and cryptographic material
  // =========================================================================

  /**
   * JWT tokens (JSON Web Tokens)
   * @example "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
   */
  {
    name: "jwt_token",
    pattern: /eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*/g,
    replacement: "[JWT_TOKEN]",
  },

  /**
   * RSA private keys (PEM format)
   * @example "-----BEGIN RSA PRIVATE KEY-----\nMIIE..."
   */
  {
    name: "rsa_private_key",
    pattern:
      /-----BEGIN\s+(?:RSA\s+)?PRIVATE\s+KEY-----[\s\S]*?-----END\s+(?:RSA\s+)?PRIVATE\s+KEY-----/g,
    replacement: "[PRIVATE_KEY]",
  },

  /**
   * EC private keys (PEM format)
   * @example "-----BEGIN EC PRIVATE KEY-----\nMHQC..."
   */
  {
    name: "ec_private_key",
    pattern:
      /-----BEGIN\s+(?:EC\s+)?PRIVATE\s+KEY-----[\s\S]*?-----END\s+(?:EC\s+)?PRIVATE\s+KEY-----/g,
    replacement: "[PRIVATE_KEY]",
  },

  /**
   * OpenSSH private keys
   * @example "-----BEGIN OPENSSH PRIVATE KEY-----\nb3Bl..."
   */
  {
    name: "openssh_private_key",
    pattern:
      /-----BEGIN\s+OPENSSH\s+PRIVATE\s+KEY-----[\s\S]*?-----END\s+OPENSSH\s+PRIVATE\s+KEY-----/g,
    replacement: "[SSH_PRIVATE_KEY]",
  },

  /**
   * Generic PEM private keys (DSA, PKCS8, etc.)
   * @example "-----BEGIN PRIVATE KEY-----", "-----BEGIN DSA PRIVATE KEY-----"
   */
  {
    name: "generic_private_key",
    pattern:
      /-----BEGIN\s+[A-Z ]+\s+PRIVATE\s+KEY-----[\s\S]*?-----END\s+[A-Z ]+\s+PRIVATE\s+KEY-----/g,
    replacement: "[PRIVATE_KEY]",
  },

  // =========================================================================
  // PII patterns
  // =========================================================================

  /**
   * Email addresses
   * @example "user@example.com", "john.doe+tag@company.co.uk"
   */
  {
    name: "email",
    pattern: /[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/g,
    replacement: "[EMAIL]",
  },

  /**
   * US Social Security Numbers (SSN)
   * @example "123-45-6789", "123 45 6789", "123456789"
   */
  {
    name: "ssn",
    pattern: /\b\d{3}[-\s]?\d{2}[-\s]?\d{4}\b/g,
    replacement: "[SSN]",
  },

  /**
   * Credit card numbers - Visa (starts with 4)
   * @example "4111111111111111", "4111-1111-1111-1111"
   */
  {
    name: "credit_card_visa",
    pattern: /\b4\d{3}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b/g,
    replacement: "[CREDIT_CARD]",
  },

  /**
   * Credit card numbers - Mastercard (starts with 5[1-5] or 2[2-7])
   * @example "5111111111111118", "5111-1111-1111-1118"
   */
  {
    name: "credit_card_mastercard",
    pattern: /\b5[1-5]\d{2}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b/g,
    replacement: "[CREDIT_CARD]",
  },

  /**
   * Credit card numbers - American Express (starts with 34 or 37)
   * @example "371449635398431", "3714-496353-98431"
   */
  {
    name: "credit_card_amex",
    pattern: /\b3[47]\d{2}[-\s]?\d{6}[-\s]?\d{5}\b/g,
    replacement: "[CREDIT_CARD]",
  },

  /**
   * Credit card numbers - Discover (starts with 6011, 65, or 644-649)
   * @example "6011111111111117", "6011-1111-1111-1117"
   */
  {
    name: "credit_card_discover",
    pattern: /\b6(?:011|5\d{2}|4[4-9]\d)[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b/g,
    replacement: "[CREDIT_CARD]",
  },

  /**
   * US phone numbers (various formats)
   * @example "+1 (555) 123-4567", "555-123-4567", "5551234567"
   */
  {
    name: "phone_us",
    pattern: /(?:\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b/g,
    replacement: "[PHONE]",
  },

  /**
   * International phone numbers (E.164 format and common variations)
   * @example "+44 20 7946 0958", "+33 1 42 68 53 00"
   */
  {
    name: "phone_international",
    pattern: /\+[1-9]\d{0,2}[-.\s]?\(?\d{1,4}\)?[-.\s]?\d{1,4}[-.\s]?\d{1,9}/g,
    replacement: "[PHONE]",
  },

  /**
   * IPv4 addresses
   * @example "192.168.1.1", "10.0.0.255", "172.16.0.1"
   */
  {
    name: "ipv4",
    pattern:
      /\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\b/g,
    replacement: "[IP_ADDR]",
  },

  /**
   * IPv6 addresses (full format)
   * @example "2001:0db8:85a3:0000:0000:8a2e:0370:7334"
   */
  {
    name: "ipv6_full",
    pattern: /\b(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\b/g,
    replacement: "[IPV6_ADDR]",
  },

  /**
   * IPv6 addresses (compressed with trailing ::)
   * @example "2001:db8::", "fe80::"
   */
  {
    name: "ipv6_compressed_trailing",
    pattern: /\b(?:[0-9a-fA-F]{1,4}:){1,7}:\b/g,
    replacement: "[IPV6_ADDR]",
  },

  /**
   * IPv6 addresses (compressed with leading ::)
   * @example "::1", "::ffff:192.168.1.1"
   */
  {
    name: "ipv6_compressed_leading",
    pattern: /\b::(?:[0-9a-fA-F]{1,4}:){0,6}[0-9a-fA-F]{1,4}\b/g,
    replacement: "[IPV6_ADDR]",
  },

  /**
   * IPv6 addresses (compressed in middle)
   * @example "2001:db8::8a2e:370:7334"
   */
  {
    name: "ipv6_compressed_middle",
    pattern:
      /\b(?:[0-9a-fA-F]{1,4}:){1,6}:(?:[0-9a-fA-F]{1,4}:){0,5}[0-9a-fA-F]{1,4}\b/g,
    replacement: "[IPV6_ADDR]",
  },

  // =========================================================================
  // File system paths
  // =========================================================================

  /**
   * Unix home directory paths
   * @example "/home/johndoe/projects", "/home/user/.ssh"
   */
  {
    name: "unix_home_path",
    pattern: /\/home\/[^/\s]+/g,
    replacement: "/home/[USER]",
  },

  /**
   * macOS home directory paths
   * @example "/Users/johndoe/Documents", "/Users/admin/.config"
   */
  {
    name: "macos_home_path",
    pattern: /\/Users\/[^/\s]+/g,
    replacement: "/Users/[USER]",
  },

  /**
   * Windows home directory paths
   * @example "C:\Users\JohnDoe\Documents", "C:\Users\Admin"
   */
  {
    name: "windows_home_path",
    pattern: /C:\\Users\\[^\\\s]+/gi,
    replacement: "C:\\Users\\[USER]",
  },

  // =========================================================================
  // Connection strings and URLs
  // =========================================================================

  /**
   * Database connection strings
   * @example "mongodb://user:pass@host:27017/db", "postgres://admin:secret@localhost/mydb"
   */
  {
    name: "db_connection_string",
    pattern: /(mongodb|postgres|mysql|redis|amqp):\/\/[^\s]+/gi,
    replacement: "$1://[CONNECTION_STRING]",
  },

  /**
   * URLs with embedded credentials
   * @example "https://user:password@api.example.com/path"
   */
  {
    name: "url_with_credentials",
    pattern: /(https?):\/\/[^:]+:[^@]+@/g,
    replacement: "$1://[CREDENTIALS]@",
  },

  // =========================================================================
  // Generic patterns (last resort, may have false positives)
  // =========================================================================

  /**
   * Generic API keys (long alphanumeric strings)
   * @example "abcdef123456789012345678901234567890"
   */
  {
    name: "generic_api_key",
    pattern: /\b[A-Za-z0-9_-]{32,}\b/g,
    replacement: "[API_KEY]",
  },

  /**
   * Generic long hex strings (often secrets or hashes)
   * @example "0123456789abcdef0123456789abcdef"
   */
  {
    name: "hex_string",
    pattern: /\b[a-fA-F0-9]{32,}\b/g,
    replacement: "[HEX_STRING]",
  },

  /**
   * Base64 encoded strings with padding (likely secrets)
   * @example "SGVsbG8gV29ybGQhIFRoaXMgaXMgYSBzZWNyZXQ="
   */
  {
    name: "base64_string",
    pattern: /\b[A-Za-z0-9+/]{40,}={1,2}\b/g,
    replacement: "[BASE64_STRING]",
  },
];

/**
 * Legacy sensitivePatterns array for backward compatibility.
 * @deprecated Use redactionPatterns instead.
 */
const sensitivePatterns: readonly Sanitizer[] = redactionPatterns.map(
  ({ pattern, replacement }) => ({ pattern, replacement })
);

/**
 * File extensions that should have their paths redacted while preserving structure.
 */
const fileExtensions = [
  ".go",
  ".ts",
  ".tsx",
  ".js",
  ".jsx",
  ".py",
  ".rs",
  ".java",
  ".c",
  ".cpp",
  ".h",
  ".hpp",
  ".cs",
  ".rb",
  ".php",
  ".swift",
  ".kt",
  ".scala",
  ".vue",
  ".svelte",
] as const;

/**
 * Pre-compiled regex for file path redaction.
 * Matches paths ending with known extensions, capturing the extension for preservation.
 * Performance: Single regex pass instead of O(extensions * occurrences) loop.
 */
const filePathPattern = new RegExp(
  `(?<=[\\s"'\`(\\[]|^)[^\\s"'\`()\\[\\]]+?(${fileExtensions.map((ext) => ext.replace(".", "\\.")).join("|")})(?=[\\s"'\`()\\]:]|$|:)`,
  "g"
);

// ============================================================================
// Public Functions
// ============================================================================

/**
 * Redact sensitive data from text using all sensitive patterns.
 * This is used for general text sanitization.
 */
export const redactSensitiveData = (text: string): string => {
  let result = text;

  for (const sanitizer of sensitivePatterns) {
    result = result.replace(sanitizer.pattern, sanitizer.replacement);
  }

  return result;
};

/**
 * Redact PII and sensitive data from arbitrary text.
 * Applies all redaction patterns to detect and replace sensitive information.
 *
 * This function is optimized for performance:
 * - Patterns are compiled once at module load time (not per-call)
 * - Patterns are applied in order of specificity (most specific first)
 *
 * @param text - The text to redact
 * @returns Text with all detected PII and secrets replaced with placeholders
 *
 * @example
 * ```typescript
 * const input = "Contact user@example.com or call 555-123-4567";
 * const output = redactPII(input);
 * // output: "Contact [EMAIL] or call [PHONE]"
 * ```
 */
export const redactPII = (text: string): string => {
  let result = text;

  for (const { pattern, replacement } of redactionPatterns) {
    result = result.replace(pattern, replacement);
  }

  return result;
};

/**
 * Redact sensitive data from an ExtractedError object.
 * Creates a new error with all string fields sanitized.
 *
 * Redacts PII from:
 * - message
 * - raw
 * - stackTrace
 * - file (user paths)
 * - suggestions
 * - codeSnippet lines
 *
 * @param error - The error to redact
 * @returns A new ExtractedError with sensitive data removed
 *
 * @example
 * ```typescript
 * const error: ExtractedError = {
 *   message: "Failed to connect to user@host.com",
 *   file: "/Users/johndoe/project/src/app.ts",
 *   line: 42,
 * };
 * const redacted = redactErrorMessage(error);
 * // redacted.message: "Failed to connect to [EMAIL]"
 * // redacted.file: "/Users/[USER]/project/src/app.ts"
 * ```
 */
export const redactErrorMessage = (error: ExtractedError): ExtractedError => {
  const redacted: ExtractedError = {
    ...error,
    message: redactPII(error.message),
    raw: error.raw ? redactPII(error.raw) : undefined,
    stackTrace: error.stackTrace ? redactPII(error.stackTrace) : undefined,
    file: error.file ? redactPII(error.file) : undefined,
    suggestions: error.suggestions
      ? error.suggestions.map((s) => redactPII(s))
      : undefined,
    codeSnippet: error.codeSnippet
      ? {
          ...error.codeSnippet,
          lines: error.codeSnippet.lines.map((line) => redactPII(line)),
        }
      : undefined,
  };

  return redacted;
};

/**
 * Redact sensitive data from an entire ErrorReport.
 * Creates a new report with all errors sanitized while preserving statistics.
 *
 * Note: Statistics (byFile counts, etc.) are NOT redacted as they may contain
 * file paths that users need for navigation. Use sanitizeForTelemetry if you
 * need to redact file paths for external transmission.
 *
 * @param report - The error report to redact
 * @returns A new ErrorReport with sensitive data removed from all errors
 *
 * @example
 * ```typescript
 * const report = createErrorReport(errors);
 * const redactedReport = redactReport(report);
 * // All error messages, stack traces, etc. are now redacted
 * ```
 */
export const redactReport = (report: ErrorReport): ErrorReport => ({
  ...report,
  errors: report.errors.map(redactErrorMessage),
  aiContext: report.aiContext
    ? {
        ...report.aiContext,
        commitSha: report.aiContext.commitSha
          ? redactPII(report.aiContext.commitSha)
          : undefined,
        treeHash: report.aiContext.treeHash
          ? redactPII(report.aiContext.treeHash)
          : undefined,
        repoRoot: report.aiContext.repoRoot
          ? redactPII(report.aiContext.repoRoot)
          : undefined,
      }
    : undefined,
});

/**
 * Sanitize a pattern for telemetry by removing potentially sensitive information.
 * It preserves the structure of the error (file extensions, line/column numbers, keywords)
 * while removing actual file paths, credentials, and other PII.
 *
 * SECURITY: This function is critical for preventing sensitive data from being sent to telemetry.
 * When in doubt, redact more rather than less.
 */
export const sanitizeForTelemetry = (pattern: string): string => {
  // Limit length to prevent excessive data
  let result =
    pattern.length > maxPatternLineLength
      ? `${pattern.slice(0, maxPatternLineLength)}...`
      : pattern;

  // Replace sensitive patterns using regex
  for (const sanitizer of sensitivePatterns) {
    result = result.replace(sanitizer.pattern, sanitizer.replacement);
  }

  // Replace file paths with placeholders while keeping the extension
  // Single regex pass for all extensions (performance optimization)
  result = result.replace(
    filePathPattern,
    (_match, ext: string) => `[path]${ext}`
  );

  return result;
};
