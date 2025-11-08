package clierr

import (
	"errors"
	"testing"
)

func TestError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *Error
		wantMsg string
	}{
		{
			name:    "simple error message",
			err:     New(Validation, "invalid input", nil),
			wantMsg: "invalid input",
		},
		{
			name:    "error with underlying error",
			err:     New(Download, "download failed", errors.New("network timeout")),
			wantMsg: "download failed",
		},
		{
			name:    "empty message",
			err:     New(Internal, "", nil),
			wantMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.wantMsg {
				t.Errorf("Error() = %v, want %v", got, tt.wantMsg)
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	tests := []struct {
		name    string
		err     *Error
		wantNil bool
	}{
		{
			name:    "no underlying error",
			err:     New(Validation, "test", nil),
			wantNil: true,
		},
		{
			name:    "with underlying error",
			err:     New(Download, "test", errors.New("underlying")),
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Unwrap()
			if (got == nil) != tt.wantNil {
				t.Errorf("Unwrap() nil = %v, want nil = %v", got == nil, tt.wantNil)
			}
		})
	}
}

func TestError_UnwrapChain(t *testing.T) {
	wrappedErr := errors.New("wrapped: root cause")
	cliErr := New(Internal, "cli error", wrappedErr)

	// Test that error.Is works with our Unwrap
	if !errors.Is(cliErr, wrappedErr) {
		t.Error("errors.Is should find wrapped error")
	}

	// Test unwrap directly
	unwrapped := cliErr.Unwrap()
	if unwrapped != wrappedErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, wrappedErr)
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		errorType   Type
		message     string
		underlying  error
		wantType    Type
		wantMessage string
		wantErr     bool
	}{
		{
			name:        "validation error",
			errorType:   Validation,
			message:     "invalid game ID",
			underlying:  nil,
			wantType:    Validation,
			wantMessage: "invalid game ID",
			wantErr:     false,
		},
		{
			name:        "not found error",
			errorType:   NotFound,
			message:     "game not found",
			underlying:  errors.New("sql: no rows"),
			wantType:    NotFound,
			wantMessage: "game not found",
			wantErr:     true,
		},
		{
			name:        "download error",
			errorType:   Download,
			message:     "failed to download",
			underlying:  errors.New("connection reset"),
			wantType:    Download,
			wantMessage: "failed to download",
			wantErr:     true,
		},
		{
			name:        "internal error",
			errorType:   Internal,
			message:     "unexpected error",
			underlying:  errors.New("panic recovered"),
			wantType:    Internal,
			wantMessage: "unexpected error",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := New(tt.errorType, tt.message, tt.underlying)

			if got.Type != tt.wantType {
				t.Errorf("New().Type = %v, want %v", got.Type, tt.wantType)
			}

			if got.Message != tt.wantMessage {
				t.Errorf("New().Message = %v, want %v", got.Message, tt.wantMessage)
			}

			if (got.Err != nil) != tt.wantErr {
				t.Errorf("New().Err nil = %v, want nil = %v", got.Err == nil, !tt.wantErr)
			}

			if tt.wantErr && got.Err != tt.underlying {
				t.Errorf("New().Err = %v, want %v", got.Err, tt.underlying)
			}
		})
	}
}

func TestError_Types(t *testing.T) {
	// Test that all type constants are defined correctly
	types := []Type{Validation, NotFound, Download, Internal}
	expected := []string{"validation", "not_found", "download", "internal"}

	for i, typ := range types {
		if string(typ) != expected[i] {
			t.Errorf("Type constant = %v, want %v", typ, expected[i])
		}
	}
}

func TestError_NilUnderlying(t *testing.T) {
	err := New(Validation, "test", nil)

	// Should not panic when calling Unwrap with nil underlying
	got := err.Unwrap()
	if got != nil {
		t.Errorf("Unwrap() with nil underlying = %v, want nil", got)
	}
}

func TestError_ErrorInterface(t *testing.T) {
	// Test that Error implements error interface
	var _ error = (*Error)(nil)

	err := New(Validation, "test message", nil)
	var e error = err

	if e.Error() != "test message" {
		t.Errorf("Error interface Error() = %v, want %v", e.Error(), "test message")
	}
}

func TestError_ErrorsIsAs(t *testing.T) {
	underlyingErr := errors.New("underlying error")
	cliErr := New(Download, "download failed", underlyingErr)

	// Test errors.Is
	if !errors.Is(cliErr, underlyingErr) {
		t.Error("errors.Is should find underlying error")
	}

	// Test errors.As
	var cliErrTarget *Error
	if !errors.As(cliErr, &cliErrTarget) {
		t.Error("errors.As should find Error type")
	}

	if cliErrTarget.Type != Download {
		t.Errorf("errors.As Type = %v, want %v", cliErrTarget.Type, Download)
	}
}

func TestError_ChainedErrors(t *testing.T) {
	// Test error chain unwrapping
	wrappedErr := errors.New("level 1")
	cliErr := New(Internal, "level 2", wrappedErr)

	// Unwrap once
	unwrapped := cliErr.Unwrap()
	if unwrapped != wrappedErr {
		t.Errorf("First Unwrap() = %v, want %v", unwrapped, wrappedErr)
	}
}

func TestError_EmptyType(t *testing.T) {
	// Test with an empty type string
	err := New(Type(""), "message", nil)
	if err.Type != Type("") {
		t.Errorf("Empty type = %v, want empty string", err.Type)
	}
}

func TestError_LongMessage(t *testing.T) {
	// Test with a very long message
	longMsg := string(make([]byte, 10000))
	err := New(Validation, longMsg, nil)

	if err.Message != longMsg {
		t.Error("Long message not preserved")
	}
}

func TestError_SpecialCharacters(t *testing.T) {
	// Test with special characters in a message
	specialMsg := "Error: 测试\n\t\"quotes\" 'apostrophes' <brackets> & symbols!"
	err := New(Validation, specialMsg, nil)

	if err.Error() != specialMsg {
		t.Errorf("Special characters not preserved: got %q, want %q", err.Error(), specialMsg)
	}
}
