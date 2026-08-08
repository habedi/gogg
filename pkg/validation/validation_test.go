package validation

import (
	"testing"
)

func TestValidateThreadCount(t *testing.T) {
	tests := []struct {
		name    string
		threads int
		wantErr bool
	}{
		{"valid minimum", 1, false},
		{"valid middle", 10, false},
		{"valid maximum", 20, false},
		{"too low", 0, true},
		{"negative", -1, true},
		{"too high", 21, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateThreadCount(tt.threads)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateThreadCount(%d) error = %v, wantErr %v", tt.threads, err, tt.wantErr)
			}
		})
	}
}

func TestValidateGameID(t *testing.T) {
	tests := []struct {
		name    string
		id      int
		wantErr bool
	}{
		{"valid positive", 123, false},
		{"valid large", 999999, false},
		{"zero", 0, true},
		{"negative", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGameID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGameID(%d) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestValidateNonEmptyString(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		value     string
		wantErr   bool
	}{
		{"valid string", "username", "john", false},
		{"empty string", "username", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNonEmptyString(tt.fieldName, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNonEmptyString(%q, %q) error = %v, wantErr %v", tt.fieldName, tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		wantErr  bool
	}{
		{"all platforms", "all", false},
		{"windows", "windows", false},
		{"mac", "mac", false},
		{"linux", "linux", false},
		{"invalid", "ios", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlatform(tt.platform)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePlatform(%q) error = %v, wantErr %v", tt.platform, err, tt.wantErr)
			}
		})
	}
}

func TestValidateConnectionCount(t *testing.T) {
	for _, n := range []int{MinConnections, 4, MaxConnections} {
		if err := ValidateConnectionCount(n); err != nil {
			t.Errorf("ValidateConnectionCount(%d) unexpected error: %v", n, err)
		}
	}
	for _, n := range []int{0, -1, MaxConnections + 1} {
		if err := ValidateConnectionCount(n); err == nil {
			t.Errorf("ValidateConnectionCount(%d) expected an error", n)
		}
	}
}

func TestValidateLanguageCode(t *testing.T) {
	codes := map[string]string{"en": "English", "de": "Deutsch"}
	if err := ValidateLanguageCode("en", codes); err != nil {
		t.Errorf("known code should validate: %v", err)
	}
	if err := ValidateLanguageCode("xx", codes); err == nil {
		t.Error("unknown code should fail")
	}
	if err := ValidateLanguageCode("", codes); err == nil {
		t.Error("empty code should fail")
	}
}
