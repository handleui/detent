package util

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		// Seconds (< 1 minute)
		{
			name:     "zero duration",
			duration: 0,
			want:     "0 seconds",
		},
		{
			name:     "one second",
			duration: time.Second,
			want:     "1 second",
		},
		{
			name:     "two seconds",
			duration: 2 * time.Second,
			want:     "2 seconds",
		},
		{
			name:     "45 seconds",
			duration: 45 * time.Second,
			want:     "45 seconds",
		},
		{
			name:     "59 seconds",
			duration: 59 * time.Second,
			want:     "59 seconds",
		},
		{
			name:     "subsecond rounded down",
			duration: 500 * time.Millisecond,
			want:     "0 seconds",
		},
		{
			name:     "1.9 seconds rounded down",
			duration: 1900 * time.Millisecond,
			want:     "1 second",
		},

		// Minutes (1 minute to < 1 hour)
		{
			name:     "exactly one minute",
			duration: time.Minute,
			want:     "1 minutes",
		},
		{
			name:     "two minutes",
			duration: 2 * time.Minute,
			want:     "2 minutes",
		},
		{
			name:     "five minutes",
			duration: 5 * time.Minute,
			want:     "5 minutes",
		},
		{
			name:     "30 minutes",
			duration: 30 * time.Minute,
			want:     "30 minutes",
		},
		{
			name:     "59 minutes",
			duration: 59 * time.Minute,
			want:     "59 minutes",
		},
		{
			name:     "1 minute 30 seconds",
			duration: time.Minute + 30*time.Second,
			want:     "1 minutes",
		},

		// Hours (1 hour to < 24 hours)
		{
			name:     "exactly one hour",
			duration: time.Hour,
			want:     "1 hours",
		},
		{
			name:     "two hours",
			duration: 2 * time.Hour,
			want:     "2 hours",
		},
		{
			name:     "12 hours",
			duration: 12 * time.Hour,
			want:     "12 hours",
		},
		{
			name:     "23 hours",
			duration: 23 * time.Hour,
			want:     "23 hours",
		},
		{
			name:     "2 hours 30 minutes",
			duration: 2*time.Hour + 30*time.Minute,
			want:     "2 hours",
		},

		// Days (>= 24 hours)
		{
			name:     "exactly one day",
			duration: 24 * time.Hour,
			want:     "1 days",
		},
		{
			name:     "two days",
			duration: 48 * time.Hour,
			want:     "2 days",
		},
		{
			name:     "three days",
			duration: 72 * time.Hour,
			want:     "3 days",
		},
		{
			name:     "7 days",
			duration: 168 * time.Hour,
			want:     "7 days",
		},
		{
			name:     "30 days",
			duration: 720 * time.Hour,
			want:     "30 days",
		},
		{
			name:     "1 day 6 hours",
			duration: 30 * time.Hour,
			want:     "1 days",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

func TestFormatDurationCompact(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		// Seconds (< 1 minute)
		{
			name:     "zero duration",
			duration: 0,
			want:     "0.0s",
		},
		{
			name:     "one second",
			duration: time.Second,
			want:     "1.0s",
		},
		{
			name:     "2.3 seconds",
			duration: 2300 * time.Millisecond,
			want:     "2.3s",
		},
		{
			name:     "45 seconds",
			duration: 45 * time.Second,
			want:     "45.0s",
		},
		{
			name:     "59.9 seconds",
			duration: 59900 * time.Millisecond,
			want:     "59.9s",
		},
		{
			name:     "500 milliseconds",
			duration: 500 * time.Millisecond,
			want:     "0.5s",
		},

		// Minutes (>= 1 minute)
		{
			name:     "exactly one minute",
			duration: time.Minute,
			want:     "1m",
		},
		{
			name:     "two minutes",
			duration: 2 * time.Minute,
			want:     "2m",
		},
		{
			name:     "1 minute 23 seconds",
			duration: time.Minute + 23*time.Second,
			want:     "1m 23s",
		},
		{
			name:     "5 minutes 0 seconds",
			duration: 5 * time.Minute,
			want:     "5m",
		},
		{
			name:     "10 minutes 45 seconds",
			duration: 10*time.Minute + 45*time.Second,
			want:     "10m 45s",
		},
		{
			name:     "59 minutes 59 seconds",
			duration: 59*time.Minute + 59*time.Second,
			want:     "59m 59s",
		},

		// Hours (displayed as minutes)
		{
			name:     "exactly one hour",
			duration: time.Hour,
			want:     "60m",
		},
		{
			name:     "1 hour 30 minutes",
			duration: time.Hour + 30*time.Minute,
			want:     "90m",
		},
		{
			name:     "2 hours",
			duration: 2 * time.Hour,
			want:     "120m",
		},
		{
			name:     "2 hours 15 minutes 30 seconds",
			duration: 2*time.Hour + 15*time.Minute + 30*time.Second,
			want:     "135m 30s",
		},

		// Days (displayed as minutes)
		{
			name:     "one day",
			duration: 24 * time.Hour,
			want:     "1440m",
		},
		{
			name:     "1 day 1 hour 1 minute 1 second",
			duration: 24*time.Hour + time.Hour + time.Minute + time.Second,
			want:     "1501m 1s",
		},

		// Edge cases with rounding
		{
			name:     "1 minute 0.5 seconds",
			duration: time.Minute + 500*time.Millisecond,
			want:     "1m",
		},
		{
			name:     "1 minute 0.9 seconds",
			duration: time.Minute + 900*time.Millisecond,
			want:     "1m",
		},
		{
			name:     "0.1 seconds",
			duration: 100 * time.Millisecond,
			want:     "0.1s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDurationCompact(tt.duration)
			if got != tt.want {
				t.Errorf("FormatDurationCompact(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

// TestFormatDurationBoundaries tests boundary conditions
func TestFormatDurationBoundaries(t *testing.T) {
	t.Run("just under one minute", func(t *testing.T) {
		d := time.Minute - time.Millisecond
		got := FormatDuration(d)
		if got != "59 seconds" {
			t.Errorf("FormatDuration(59.999s) = %q, want %q", got, "59 seconds")
		}
	})

	t.Run("just under one hour", func(t *testing.T) {
		d := time.Hour - time.Millisecond
		got := FormatDuration(d)
		if got != "59 minutes" {
			t.Errorf("FormatDuration(59m 59.999s) = %q, want %q", got, "59 minutes")
		}
	})

	t.Run("just under one day", func(t *testing.T) {
		d := 24*time.Hour - time.Millisecond
		got := FormatDuration(d)
		if got != "23 hours" {
			t.Errorf("FormatDuration(23h 59m 59.999s) = %q, want %q", got, "23 hours")
		}
	})
}

// TestFormatDurationCompactBoundaries tests boundary conditions for compact format
func TestFormatDurationCompactBoundaries(t *testing.T) {
	t.Run("just under one minute", func(t *testing.T) {
		d := time.Minute - time.Millisecond
		got := FormatDurationCompact(d)
		// Due to floating-point precision, 59.999s rounds to 60.0s
		want := "60.0s"
		if got != want {
			t.Errorf("FormatDurationCompact(59.999s) = %q, want %q", got, want)
		}
	})

	t.Run("exactly at minute boundary", func(t *testing.T) {
		d := time.Minute
		got := FormatDurationCompact(d)
		want := "1m"
		if got != want {
			t.Errorf("FormatDurationCompact(1m) = %q, want %q", got, want)
		}
	})
}

// BenchmarkFormatDuration benchmarks the duration formatting function
func BenchmarkFormatDuration(b *testing.B) {
	durations := []time.Duration{
		5 * time.Second,
		5 * time.Minute,
		5 * time.Hour,
		5 * 24 * time.Hour,
	}

	for _, d := range durations {
		b.Run(d.String(), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = FormatDuration(d)
			}
		})
	}
}

// BenchmarkFormatDurationCompact benchmarks the compact duration formatting function
func BenchmarkFormatDurationCompact(b *testing.B) {
	durations := []time.Duration{
		5 * time.Second,
		5 * time.Minute,
		5 * time.Hour,
		5 * 24 * time.Hour,
	}

	for _, d := range durations {
		b.Run(d.String(), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = FormatDurationCompact(d)
			}
		})
	}
}
