package validation

import (
	"strings"
	"testing"
)

func TestValidateGameID_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		gameID  int
		wantErr bool
	}{
		{"zero ID", 0, true},
		{"negative ID", -1, true},
		{"negative large", -999999, true},
		{"minimum valid", 1, false},
		{"small valid", 10, false},
		{"large valid", 999999999, false},
		{"max int", 2147483647, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGameID(tt.gameID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGameID(%d) error = %v, wantErr %v", tt.gameID, err, tt.wantErr)
			}
		})
	}
}

func TestValidateThreadCount_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		threads int
		wantErr bool
	}{
		{"zero threads", 0, true},
		{"negative threads", -1, true},
		{"minimum valid", 1, false},
		{"normal value", 5, false},
		{"maximum valid", 20, false},
		{"above maximum", 21, true},
		{"way above maximum", 100, true},
		{"very negative", -999, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateThreadCount(tt.threads)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateThreadCount(%d) error = %v, wantErr %v", tt.threads, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "thread") {
				t.Errorf("Error message should mention 'thread': %v", err)
			}
		})
	}
}

func TestValidatePlatform_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		wantErr  bool
	}{
		{"valid all", "all", false},
		{"valid windows", "windows", false},
		{"valid mac", "mac", false},
		{"valid linux", "linux", false},
		{"uppercase ALL", "ALL", true},         // Case-sensitive
		{"uppercase Windows", "Windows", true}, // Case-sensitive
		{"mixed case Mac", "Mac", true},        // Case-sensitive
		{"mixed case Linux", "LINUX", true},    // Case-sensitive
		{"invalid platform", "bsd", true},
		{"invalid android", "android", true},
		{"empty string", "", true},
		{"whitespace only", "   ", true},
		{"with spaces", " windows ", true}, // Doesn't trim
		{"partial match", "win", true},
		{"typo", "windowz", true},
		{"special chars", "windows!", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlatform(tt.platform)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePlatform(%q) error = %v, wantErr %v", tt.platform, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "platform") {
				t.Errorf("Error message should mention 'platform': %v", err)
			}
		})
	}
}

func TestValidateNonEmptyString_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"normal string", "test", false},
		{"empty string", "", true},
		{"single space", " ", false},          // Only checks empty, not whitespace
		{"multiple spaces", "   ", false},     // Only checks empty
		{"tab", "\t", false},                  // Only checks empty
		{"newline", "\n", false},              // Only checks empty
		{"mixed whitespace", " \t\n ", false}, // Only checks empty
		{"string with leading space", " test", false},
		{"string with trailing space", "test ", false},
		{"string with internal space", "test string", false},
		{"single char", "a", false},
		{"unicode", "测试", false},
		{"emoji", "🎮", false},
		{"very long string", strings.Repeat("a", 10000), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNonEmptyString("test field", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNonEmptyString(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "test field") {
				t.Errorf("Error message should mention field name: %v", err)
			}
		})
	}
}

func TestValidateNonEmptyString_FieldNames(t *testing.T) {
	// Test that field name appears in error message
	tests := []struct {
		fieldName string
	}{
		{"username"},
		{"password"},
		{"game title"},
		{"file path"},
		{""}, // Empty field name
		{"very long field name with spaces and special characters!@#"},
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			err := ValidateNonEmptyString(tt.fieldName, "")
			if err == nil {
				t.Error("Expected error for empty string")
				return
			}
			if tt.fieldName != "" && !strings.Contains(err.Error(), tt.fieldName) {
				t.Errorf("Error message should contain field name %q: %v", tt.fieldName, err)
			}
		})
	}
}

func TestValidateGameID_BoundaryValues(t *testing.T) {
	// Test boundary conditions more thoroughly
	tests := []int{
		-2147483648, // Min int32
		-1,
		0,
		1,
		2147483647, // Max int32
	}

	for _, id := range tests {
		err := ValidateGameID(id)
		if id <= 0 && err == nil {
			t.Errorf("ValidateGameID(%d) should return error for non-positive ID", id)
		}
		if id > 0 && err != nil {
			t.Errorf("ValidateGameID(%d) should not return error for positive ID: %v", id, err)
		}
	}
}

func TestValidateThreadCount_BoundaryValues(t *testing.T) {
	// Test exact boundaries
	tests := []struct {
		threads int
		valid   bool
	}{
		{0, false},
		{1, true},
		{20, true},
		{21, false},
	}

	for _, tt := range tests {
		err := ValidateThreadCount(tt.threads)
		if tt.valid && err != nil {
			t.Errorf("ValidateThreadCount(%d) should be valid: %v", tt.threads, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("ValidateThreadCount(%d) should be invalid", tt.threads)
		}
	}
}

func TestValidatePlatform_CaseSensitivity(t *testing.T) {
	// Test that validation is case-sensitive (only lowercase is valid)
	platforms := []string{"all", "windows", "mac", "linux"}

	for _, platform := range platforms {
		// Test lowercase - should be valid
		if err := ValidatePlatform(platform); err != nil {
			t.Errorf("Lowercase %q should be valid: %v", platform, err)
		}

		// Test uppercase - should be INVALID (case-sensitive)
		upper := strings.ToUpper(platform)
		if err := ValidatePlatform(upper); err == nil {
			t.Errorf("Uppercase %q should be INVALID (case-sensitive)", upper)
		}

		// Test mixed case - should be INVALID (case-sensitive)
		if len(platform) > 0 {
			mixed := strings.ToUpper(platform[:1]) + platform[1:]
			if err := ValidatePlatform(mixed); err == nil {
				t.Errorf("Mixed case %q should be INVALID (case-sensitive)", mixed)
			}
		}
	}
}

func TestValidatePlatform_InvalidCases(t *testing.T) {
	// Test various invalid inputs
	invalid := []string{
		"freebsd",
		"unix",
		"macos", // Should be "mac"
		"osx",
		"win",
		"win32",
		"win64",
		"linux32",
		"linux64",
		"darwin",
		"123",
		"@#$%",
		"all platforms",
		"windows,linux",
	}

	for _, platform := range invalid {
		if err := ValidatePlatform(platform); err == nil {
			t.Errorf("ValidatePlatform(%q) should return error", platform)
		}
	}
}

func TestValidateNonEmptyString_WhitespaceVariations(t *testing.T) {
	// Note: ValidateNonEmptyString only checks if string == "", not if it's all whitespace
	// So these should all PASS (not error) since they're not empty strings
	whitespaceStrings := []string{
		" ",
		"  ",
		"\t",
		"\n",
		"\r",
		"\r\n",
		" \t ",
		"\t\n\r",
		"　", // Unicode space (U+3000)
	}

	for _, ws := range whitespaceStrings {
		err := ValidateNonEmptyString("field", ws)
		if err != nil {
			t.Errorf("ValidateNonEmptyString(%q) should NOT error (implementation only checks empty string, not whitespace): %v", ws, err)
		}
	}
}

func TestValidation_ErrorMessages(t *testing.T) {
	// Verify error messages are helpful
	tests := []struct {
		name     string
		validate func() error
		wantIn   []string
	}{
		{
			name:     "game ID error mentions ID",
			validate: func() error { return ValidateGameID(0) },
			wantIn:   []string{"game", "ID"},
		},
		{
			name:     "thread count error mentions range",
			validate: func() error { return ValidateThreadCount(100) },
			wantIn:   []string{"thread"},
		},
		{
			name:     "platform error mentions valid platforms",
			validate: func() error { return ValidatePlatform("invalid") },
			wantIn:   []string{"platform"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validate()
			if err == nil {
				t.Error("Expected error")
				return
			}

			errMsg := strings.ToLower(err.Error())
			for _, want := range tt.wantIn {
				if !strings.Contains(errMsg, strings.ToLower(want)) {
					t.Errorf("Error message should contain %q: %v", want, err)
				}
			}
		})
	}
}

func TestValidation_ConcurrentAccess(t *testing.T) {
	// Verify validation functions are safe for concurrent use
	done := make(chan bool)

	for i := 0; i < 100; i++ {
		go func(id int) {
			ValidateGameID(id)
			ValidateThreadCount(id % 21)
			ValidatePlatform("windows")
			ValidateNonEmptyString("test", "field")
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
	// Should not panic or race
}
