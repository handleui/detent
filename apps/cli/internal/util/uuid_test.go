package util

import (
	"regexp"
	"strings"
	"testing"
)

// TestGenerateUUID tests the UUID generation function
func TestGenerateUUID(t *testing.T) {
	t.Run("generates valid UUID v4 format", func(t *testing.T) {
		uuid, err := GenerateUUID()
		if err != nil {
			t.Fatalf("GenerateUUID() returned unexpected error: %v", err)
		}

		// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
		// where y is one of [8, 9, a, b]
		uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
		if !uuidPattern.MatchString(uuid) {
			t.Errorf("GenerateUUID() = %q, does not match UUID v4 format", uuid)
		}
	})

	t.Run("generates correct length", func(t *testing.T) {
		uuid, err := GenerateUUID()
		if err != nil {
			t.Fatalf("GenerateUUID() returned unexpected error: %v", err)
		}

		// UUID format is 36 characters: 32 hex + 4 hyphens
		expectedLength := 36
		if len(uuid) != expectedLength {
			t.Errorf("GenerateUUID() length = %d, want %d", len(uuid), expectedLength)
		}
	})

	t.Run("generates lowercase hexadecimal", func(t *testing.T) {
		uuid, err := GenerateUUID()
		if err != nil {
			t.Fatalf("GenerateUUID() returned unexpected error: %v", err)
		}

		// Remove hyphens and check if all characters are lowercase hex
		cleaned := strings.ReplaceAll(uuid, "-", "")
		hexPattern := regexp.MustCompile(`^[0-9a-f]+$`)
		if !hexPattern.MatchString(cleaned) {
			t.Errorf("GenerateUUID() = %q, contains non-lowercase-hex characters", uuid)
		}
	})

	t.Run("generates version 4 UUID", func(t *testing.T) {
		uuid, err := GenerateUUID()
		if err != nil {
			t.Fatalf("GenerateUUID() returned unexpected error: %v", err)
		}

		// The version digit should be at position 14 (0-indexed)
		// Format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
		//                      ^
		if uuid[14] != '4' {
			t.Errorf("GenerateUUID() version = %c, want 4", uuid[14])
		}
	})

	t.Run("generates RFC 4122 variant", func(t *testing.T) {
		uuid, err := GenerateUUID()
		if err != nil {
			t.Fatalf("GenerateUUID() returned unexpected error: %v", err)
		}

		// The variant bits should be at position 19 (0-indexed)
		// Format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
		//                           ^
		// Should be one of [8, 9, a, b]
		variantChar := uuid[19]
		validVariants := "89ab"
		if !strings.ContainsRune(validVariants, rune(variantChar)) {
			t.Errorf("GenerateUUID() variant = %c, want one of %q", variantChar, validVariants)
		}
	})

	t.Run("generates unique UUIDs", func(t *testing.T) {
		// Generate multiple UUIDs and ensure they're all different
		iterations := 1000
		seen := make(map[string]bool, iterations)

		for i := 0; i < iterations; i++ {
			uuid, err := GenerateUUID()
			if err != nil {
				t.Fatalf("GenerateUUID() iteration %d returned error: %v", i, err)
			}

			if seen[uuid] {
				t.Errorf("GenerateUUID() generated duplicate UUID: %q", uuid)
			}
			seen[uuid] = true
		}

		if len(seen) != iterations {
			t.Errorf("GenerateUUID() generated %d unique UUIDs out of %d iterations", len(seen), iterations)
		}
	})

	t.Run("has correct hyphen positions", func(t *testing.T) {
		uuid, err := GenerateUUID()
		if err != nil {
			t.Fatalf("GenerateUUID() returned unexpected error: %v", err)
		}

		// Hyphens should be at positions 8, 13, 18, 23
		// Format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
		//         ^       ^   ^    ^   ^
		//         0       8   13   18  23
		expectedHyphenPositions := []int{8, 13, 18, 23}
		for _, pos := range expectedHyphenPositions {
			if uuid[pos] != '-' {
				t.Errorf("GenerateUUID() position %d = %c, want '-'", pos, uuid[pos])
			}
		}
	})

	t.Run("hyphen count is correct", func(t *testing.T) {
		uuid, err := GenerateUUID()
		if err != nil {
			t.Fatalf("GenerateUUID() returned unexpected error: %v", err)
		}

		hyphenCount := strings.Count(uuid, "-")
		expectedHyphens := 4
		if hyphenCount != expectedHyphens {
			t.Errorf("GenerateUUID() hyphen count = %d, want %d", hyphenCount, expectedHyphens)
		}
	})
}

// BenchmarkGenerateUUID benchmarks the UUID generation
func BenchmarkGenerateUUID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := GenerateUUID()
		if err != nil {
			b.Fatal(err)
		}
	}
}
