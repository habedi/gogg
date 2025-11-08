package clierr

// Type categorizes a CLI-facing error for consistent messaging & potential exit codes.
type Type string

const (
	Validation Type = "validation"
	NotFound   Type = "not_found"
	Download   Type = "download"
	Internal   Type = "internal"
)

// Error is a structured user-facing error.
type Error struct {
	Type    Type
	Message string
	Err     error // optional underlying error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Err }

// New constructs a new CLI Error.
func New(t Type, msg string, err error) *Error { return &Error{Type: t, Message: msg, Err: err} }
