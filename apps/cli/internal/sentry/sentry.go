package sentry

import (
	"os"
	"time"

	"github.com/getsentry/sentry-go"
)

const (
	flushTimeout = 2 * time.Second
)

// Init initializes the Sentry SDK with the given version.
// If SENTRY_DSN is not set, Sentry is disabled (no-op).
// Returns a cleanup function that should be deferred.
func Init(version string) func() {
	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		return func() {}
	}

	env := os.Getenv("SENTRY_ENVIRONMENT")
	if env == "" {
		env = "production"
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Release:          "detent@" + version,
		Environment:      env,
		AttachStacktrace: true,
		SampleRate:       1.0,
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
// then re-panics. Use with defer at top-level entry points.
func RecoverAndPanic() {
	if r := recover(); r != nil {
		sentry.CurrentHub().Recover(r)
		sentry.Flush(flushTimeout)
		panic(r)
	}
}

// AddBreadcrumb adds context for debugging.
func AddBreadcrumb(category, message string) {
	sentry.AddBreadcrumb(&sentry.Breadcrumb{
		Category: category,
		Message:  message,
		Level:    sentry.LevelInfo,
	})
}

// SetUser sets user context for error tracking.
func SetUser(id string) {
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetUser(sentry.User{ID: id})
	})
}

// SetTag sets a tag for filtering errors.
func SetTag(key, value string) {
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetTag(key, value)
	})
}
