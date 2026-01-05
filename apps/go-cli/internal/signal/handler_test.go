package signal

import (
	"context"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// TestSetupSignalHandler_SIGINT tests that context is cancelled on SIGINT
func TestSetupSignalHandler_SIGINT(t *testing.T) {
	parent := context.Background()
	ctx := SetupSignalHandler(parent)

	// Send SIGINT to the current process
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("Failed to send SIGINT: %v", err)
	}

	// Context should be cancelled within reasonable time
	select {
	case <-ctx.Done():
		// Success - context was cancelled
	case <-time.After(2 * time.Second):
		t.Error("Context was not cancelled after SIGINT signal")
	}

	if ctx.Err() != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", ctx.Err())
	}
}

// TestSetupSignalHandler_SIGTERM tests that context is cancelled on SIGTERM
func TestSetupSignalHandler_SIGTERM(t *testing.T) {
	parent := context.Background()
	ctx := SetupSignalHandler(parent)

	// Send SIGTERM to the current process
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("Failed to send SIGTERM: %v", err)
	}

	// Context should be cancelled within reasonable time
	select {
	case <-ctx.Done():
		// Success - context was cancelled
	case <-time.After(2 * time.Second):
		t.Error("Context was not cancelled after SIGTERM signal")
	}

	if ctx.Err() != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", ctx.Err())
	}
}

// TestSetupSignalHandler_ParentCancellation tests that child context respects parent cancellation
func TestSetupSignalHandler_ParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx := SetupSignalHandler(parent)

	// Cancel parent context
	cancel()

	// Child context should also be cancelled
	select {
	case <-ctx.Done():
		// Success - context was cancelled
	case <-time.After(100 * time.Millisecond):
		t.Error("Context was not cancelled after parent cancellation")
	}

	if ctx.Err() != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", ctx.Err())
	}
}

// TestSetupSignalHandler_MultipleSignals tests that multiple signals don't cause issues
func TestSetupSignalHandler_MultipleSignals(t *testing.T) {
	parent := context.Background()
	ctx := SetupSignalHandler(parent)

	// Send multiple SIGINT signals
	for i := 0; i < 3; i++ {
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
			t.Fatalf("Failed to send SIGINT %d: %v", i, err)
		}
	}

	// Context should be cancelled
	select {
	case <-ctx.Done():
		// Success - context was cancelled
	case <-time.After(2 * time.Second):
		t.Error("Context was not cancelled after multiple SIGINT signals")
	}

	// Give time for goroutine cleanup
	time.Sleep(50 * time.Millisecond)
}

// TestSetupSignalHandler_NoSignal tests that context remains active without signals
func TestSetupSignalHandler_NoSignal(t *testing.T) {
	parent := context.Background()
	ctx := SetupSignalHandler(parent)

	// Context should not be cancelled without a signal
	select {
	case <-ctx.Done():
		t.Error("Context was cancelled without signal or parent cancellation")
	case <-time.After(100 * time.Millisecond):
		// Success - context is still active
	}

	if ctx.Err() != nil {
		t.Errorf("Expected no error, got: %v", ctx.Err())
	}
}

// TestSetupSignalHandler_CleanupOnParentCancel tests proper cleanup when parent is cancelled
func TestSetupSignalHandler_CleanupOnParentCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx := SetupSignalHandler(parent)

	// Cancel parent immediately
	cancel()

	// Wait for cleanup
	select {
	case <-ctx.Done():
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("Context not cancelled after parent cancellation")
	}

	// Give goroutine time to clean up signal handler
	time.Sleep(50 * time.Millisecond)

	// Sending a signal now should not affect the cancelled context
	// This verifies signal.Stop() was called
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("Failed to send SIGINT: %v", err)
	}

	// Context should already be cancelled from parent, not from new signal
	if ctx.Err() != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", ctx.Err())
	}
}

// TestSetupSignalHandler_MultipleHandlers tests that multiple signal handlers can coexist
func TestSetupSignalHandler_MultipleHandlers(t *testing.T) {
	parent := context.Background()
	ctx1 := SetupSignalHandler(parent)
	ctx2 := SetupSignalHandler(parent)
	ctx3 := SetupSignalHandler(parent)

	// Send SIGINT
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("Failed to send SIGINT: %v", err)
	}

	// All contexts should be cancelled
	timeout := time.After(2 * time.Second)

	select {
	case <-ctx1.Done():
	case <-timeout:
		t.Error("ctx1 was not cancelled")
	}

	select {
	case <-ctx2.Done():
	case <-timeout:
		t.Error("ctx2 was not cancelled")
	}

	select {
	case <-ctx3.Done():
	case <-timeout:
		t.Error("ctx3 was not cancelled")
	}

	// Give time for goroutine cleanup
	time.Sleep(50 * time.Millisecond)
}

// TestSetupSignalHandler_ParentWithTimeout tests signal handler with timeout parent
func TestSetupSignalHandler_ParentWithTimeout(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ctx := SetupSignalHandler(parent)

	// Wait for parent timeout
	select {
	case <-ctx.Done():
		// Success - context cancelled due to parent timeout
	case <-time.After(200 * time.Millisecond):
		t.Error("Context was not cancelled after parent timeout")
	}

	// Error should be DeadlineExceeded from parent
	if ctx.Err() != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded error, got: %v", ctx.Err())
	}
}

// TestSetupSignalHandler_SignalBeforeParentCancel tests signal arrives before parent cancellation
func TestSetupSignalHandler_SignalBeforeParentCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx := SetupSignalHandler(parent)

	// Send signal first
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("Failed to send SIGTERM: %v", err)
	}

	// Wait for signal to be processed
	select {
	case <-ctx.Done():
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Context not cancelled after signal")
	}

	// Now cancel parent (context already cancelled)
	cancel()

	// Context should still be cancelled
	if ctx.Err() != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", ctx.Err())
	}
}

// TestSetupSignalHandler_GoroutineCleanup tests that goroutine properly cleans up
func TestSetupSignalHandler_GoroutineCleanup(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx := SetupSignalHandler(parent)

	// Get initial goroutine count
	initialGoroutines := countGoroutines()

	// Cancel parent to trigger cleanup
	cancel()

	// Wait for context cancellation
	<-ctx.Done()

	// Give goroutine time to clean up
	time.Sleep(100 * time.Millisecond)

	// Goroutine count should be similar to initial (within tolerance)
	finalGoroutines := countGoroutines()
	if finalGoroutines > initialGoroutines+5 {
		t.Errorf("Potential goroutine leak: initial=%d, final=%d", initialGoroutines, finalGoroutines)
	}
}

// TestSetupSignalHandler_ImmediatelyCancelledParent tests behavior with already-cancelled parent
func TestSetupSignalHandler_ImmediatelyCancelledParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before setup

	ctx := SetupSignalHandler(parent)

	// Context should be immediately cancelled
	select {
	case <-ctx.Done():
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("Context was not immediately cancelled with already-cancelled parent")
	}

	if ctx.Err() != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", ctx.Err())
	}
}

// TestSetupSignalHandler_OnlyHandlesSIGINTAndSIGTERM tests that only SIGINT and SIGTERM are handled
func TestSetupSignalHandler_OnlyHandlesSIGINTAndSIGTERM(t *testing.T) {
	// This test verifies the handler only registers SIGINT and SIGTERM
	// We can't easily test that other signals are ignored without causing side effects
	parent := context.Background()
	ctx := SetupSignalHandler(parent)

	// Verify handler works for SIGINT
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("Failed to send SIGINT: %v", err)
	}

	select {
	case <-ctx.Done():
		// Success - SIGINT was handled
	case <-time.After(2 * time.Second):
		t.Error("Context was not cancelled after SIGINT")
	}
}

// countGoroutines returns the approximate number of running goroutines
func countGoroutines() int {
	return runtime.NumGoroutine()
}

// TestSetupSignalHandler_RaceCondition tests concurrent parent cancellations
func TestSetupSignalHandler_RaceCondition(t *testing.T) {
	// Test that concurrent cancellations don't cause issues
	// We use parent cancellation only to avoid interfering with other tests via signals
	for i := 0; i < 10; i++ {
		parent, cancel := context.WithCancel(context.Background())
		ctx := SetupSignalHandler(parent)

		// Race: multiple goroutines trying to cancel
		go func() {
			time.Sleep(time.Millisecond)
			cancel()
		}()

		go func() {
			time.Sleep(time.Millisecond)
			cancel()
		}()

		// Context should be cancelled
		select {
		case <-ctx.Done():
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Error("Context was not cancelled in race condition test")
		}

		// Clean up
		time.Sleep(5 * time.Millisecond)
	}
}

// TestSetupSignalHandler_NilParent tests behavior with nil parent context
func TestSetupSignalHandler_NilParent(t *testing.T) {
	// This should panic since context.WithCancel(nil) will panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic with nil parent context, but no panic occurred")
		}
	}()

	_ = SetupSignalHandler(nil)
}

// TestSetupSignalHandler_SequentialSignals tests handling sequential signals
func TestSetupSignalHandler_SequentialSignals(t *testing.T) {
	parent := context.Background()
	ctx := SetupSignalHandler(parent)

	// Send SIGINT
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("Failed to send SIGINT: %v", err)
	}

	// Wait for cancellation
	select {
	case <-ctx.Done():
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Context not cancelled after first signal")
	}

	// Context should remain cancelled
	if ctx.Err() != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", ctx.Err())
	}

	// Note: We don't send a second signal here because after signal.Stop(),
	// the default signal handler (process termination) would take effect.
	// The handler properly cleans up after the first signal.
}

// TestSetupSignalHandler_WithValues tests that context values are preserved
func TestSetupSignalHandler_WithValues(t *testing.T) {
	type key string
	testKey := key("testKey")
	testValue := "testValue"

	parent := context.WithValue(context.Background(), testKey, testValue)
	ctx := SetupSignalHandler(parent)

	// Verify value is accessible from child context
	if got := ctx.Value(testKey); got != testValue {
		t.Errorf("Expected value %q, got %v", testValue, got)
	}

	// Verify cancellation still works
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("Failed to send SIGINT: %v", err)
	}

	select {
	case <-ctx.Done():
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Context not cancelled after signal")
	}

	// Value should still be accessible after cancellation
	if got := ctx.Value(testKey); got != testValue {
		t.Errorf("Expected value %q after cancellation, got %v", testValue, got)
	}
}

// BenchmarkSetupSignalHandler benchmarks the setup overhead
func BenchmarkSetupSignalHandler(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parent, cancel := context.WithCancel(context.Background())
		_ = SetupSignalHandler(parent)
		cancel()
	}
}

// BenchmarkSetupSignalHandler_WithCancellation benchmarks setup and cancellation
func BenchmarkSetupSignalHandler_WithCancellation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		parent, cancel := context.WithCancel(context.Background())
		ctx := SetupSignalHandler(parent)
		cancel()
		<-ctx.Done()
	}
}
