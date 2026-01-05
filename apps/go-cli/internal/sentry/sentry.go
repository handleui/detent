package sentry

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

const (
	flushTimeout      = 2 * time.Second
	httpClientTimeout = 10 * time.Second
	maxBreadcrumbs    = 20
)

// Regex patterns for PII scrubbing
var (
	// Matches common home directory patterns: /home/username, /Users/username, C:\Users\username
	homePathPattern = regexp.MustCompile(`(?i)(/home/|/Users/|C:\\Users\\)([^/\\:]+)`)
	// Matches API keys and tokens in error messages
	apiKeyPattern = regexp.MustCompile(`(?i)(sk-ant-api\d+-|sk-|api[_-]?key[=:]\s*)([A-Za-z0-9_-]{10,})`)
	// Matches email addresses
	emailPattern = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
)

// DSN is injected at build time via ldflags for production releases.
// Example: go build -ldflags "-X github.com/detent/go-cli/internal/sentry.DSN=https://..."
// Empty by default (disabled in dev builds).
var DSN string

// Init initializes the Sentry SDK with the given version.
// Uses build-time DSN for production, respects standard opt-out env vars.
// Returns a cleanup function that should be deferred.
func Init(version string) func() {
	// Standard opt-out: DO_NOT_TRACK is a cross-tool convention
	// https://consoledonottrack.com/
	if os.Getenv("DO_NOT_TRACK") == "1" || os.Getenv("DETENT_NO_TELEMETRY") == "1" {
		return func() {}
	}

	// SENTRY_DSN env var overrides build-time DSN (for self-hosters or testing)
	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		dsn = DSN // Fall back to build-time injected value
	}
	if dsn == "" {
		return func() {} // No DSN = disabled (dev builds)
	}

	env := os.Getenv("SENTRY_ENVIRONMENT")
	if env == "" {
		env = "production"
	}

	// Use OS/arch as pseudo-hostname for grouping without PII
	// This allows filtering by platform without exposing actual hostnames
	serverName := runtime.GOOS + "-" + runtime.GOARCH

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Release:          "detent@" + version,
		Environment:      env,
		ServerName:       serverName, // Platform info only, no PII
		AttachStacktrace: true,
		SampleRate:       1.0, // Capture all errors for CLI (low volume, high value)
		Debug:            env == "development",
		MaxBreadcrumbs:   maxBreadcrumbs,
		HTTPClient: &http.Client{
			Timeout: httpClientTimeout,
		},
		IgnoreErrors: []string{
			"context canceled",
			"context deadline exceeded",
			"signal: interrupt",
			"signal: terminated",
			"EOF",                   // Common stdin/stdout close
			"broken pipe",           // Common pipe close
			"connection reset",      // Network issues
			"repository not trusted", // Expected user flow, not an error
		},
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			// Filter out user-initiated interrupts at the event level
			if hint != nil && hint.OriginalException != nil {
				errMsg := hint.OriginalException.Error()
				if strings.Contains(errMsg, "interrupt") ||
					strings.Contains(errMsg, "context canceled") ||
					strings.Contains(errMsg, "terminated") ||
					strings.Contains(errMsg, "repository not trusted") ||
					strings.Contains(errMsg, "trust prompt cancelled") ||
					strings.Contains(errMsg, "API key input cancelled") ||
					strings.Contains(errMsg, "budget") { // Budget limits are expected
					return nil
				}
			}

			// Also check the event message for common CLI exit scenarios
			if event.Message != "" {
				msg := strings.ToLower(event.Message)
				if strings.Contains(msg, "interrupt") ||
					strings.Contains(msg, "context canceled") ||
					strings.Contains(msg, "cancelled") {
					return nil
				}
			}

			// Scrub PII from the event
			scrubEvent(event)

			return event
		},
		BeforeBreadcrumb: func(breadcrumb *sentry.Breadcrumb, hint *sentry.BreadcrumbHint) *sentry.Breadcrumb {
			// Scrub PII from breadcrumb messages
			breadcrumb.Message = scrubPII(breadcrumb.Message)
			return breadcrumb
		},
	})
	if err != nil {
		return func() {}
	}

	return func() {
		sentry.Flush(flushTimeout)
	}
}

// CaptureError reports an error to Sentry if initialized.
// Safe to call even if Sentry is not configured.
func CaptureError(err error) {
	if err == nil {
		return
	}
	sentry.CaptureException(err)
}

// CaptureMessage reports a message to Sentry if initialized.
func CaptureMessage(msg string) {
	sentry.CaptureMessage(msg)
}

// RecoverAndPanic recovers from a panic, reports it to Sentry,
// then re-panics so the CLI shows the panic to the user.
// IMPORTANT: This must be deferred BEFORE sentry.Init's cleanup function
// so that Flush runs before the re-panic. Example:
//
//	defer sentry.RecoverAndPanic()
//	cleanup := sentry.Init(version)
//	defer cleanup()
func RecoverAndPanic() {
	if r := recover(); r != nil {
		// Use RecoverWithContext for proper context propagation
		sentry.CurrentHub().RecoverWithContext(context.Background(), r)
		sentry.Flush(flushTimeout)
		panic(r)
	}
}

// AddBreadcrumb adds context for debugging.
func AddBreadcrumb(category, message string) {
	sentry.AddBreadcrumb(&sentry.Breadcrumb{
		Category:  category,
		Message:   message,
		Level:     sentry.LevelInfo,
		Timestamp: time.Now(),
	})
}

// SetUser sets user context for error tracking.
func SetUser(id string) {
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetUser(sentry.User{ID: id})
	})
}

// SetTag sets a tag for filtering errors.
// Values are scrubbed of PII before being set.
func SetTag(key, value string) {
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetTag(key, scrubPII(value))
	})
}

// scrubPII removes personally identifiable information from a string.
// This includes usernames in paths, API keys, and email addresses.
func scrubPII(s string) string {
	// Scrub home directory paths: /Users/john/... -> /Users/[user]/...
	s = homePathPattern.ReplaceAllString(s, "${1}[user]")

	// Scrub API keys and tokens
	s = apiKeyPattern.ReplaceAllString(s, "${1}[REDACTED]")

	// Scrub email addresses
	s = emailPattern.ReplaceAllString(s, "[email]")

	return s
}

// scrubEvent removes PII from all parts of a Sentry event.
func scrubEvent(event *sentry.Event) {
	// Scrub main message
	event.Message = scrubPII(event.Message)

	// Scrub exception messages
	for i := range event.Exception {
		event.Exception[i].Value = scrubPII(event.Exception[i].Value)

		// Scrub stack trace file paths
		if event.Exception[i].Stacktrace != nil {
			for j := range event.Exception[i].Stacktrace.Frames {
				frame := &event.Exception[i].Stacktrace.Frames[j]
				frame.AbsPath = scrubPII(frame.AbsPath)
				frame.Filename = scrubPII(frame.Filename)
			}
		}
	}

	// Scrub breadcrumb messages
	for i := range event.Breadcrumbs {
		event.Breadcrumbs[i].Message = scrubPII(event.Breadcrumbs[i].Message)
	}

	// Scrub extra data
	for key, value := range event.Extra {
		if str, ok := value.(string); ok {
			event.Extra[key] = scrubPII(str)
		}
	}

	// Scrub tag values
	for key, value := range event.Tags {
		event.Tags[key] = scrubPII(value)
	}
}
