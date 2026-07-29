// Package errs provides types and support related to web error functionality.
package errs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"

	buckyffmpeg "github.com/ardanlabs/kronk/sdk/bucky/ffmpeg"
	buckypool "github.com/ardanlabs/kronk/sdk/bucky/pool"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/hf"
	"github.com/ardanlabs/kronk/sdk/kronk/jsonrepair"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	kronkpool "github.com/ardanlabs/kronk/sdk/kronk/pool"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	"github.com/ardanlabs/kronk/sdk/tools/github"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
)

// ErrCode represents an error code in the system.
type ErrCode struct {
	value int
}

// Value returns the integer value of the error code.
func (ec ErrCode) Value() int {
	return ec.value
}

// String returns the string representation of the error code.
func (ec ErrCode) String() string {
	return codeNames[ec]
}

// UnmarshalText implement the unmarshal interface for JSON conversions.
func (ec *ErrCode) UnmarshalText(data []byte) error {
	errName := string(data)

	v, exists := codeNumbers[errName]
	if !exists {
		return fmt.Errorf("err code %q does not exist", errName)
	}

	*ec = v

	return nil
}

// MarshalText implement the marshal interface for JSON conversions.
func (ec ErrCode) MarshalText() ([]byte, error) {
	return []byte(ec.String()), nil
}

// Equal provides support for the go-cmp package and testing.
func (ec ErrCode) Equal(ec2 ErrCode) bool {
	return ec.value == ec2.value
}

// =============================================================================

// Error represents an error in the system.
type Error struct {
	Code     ErrCode `json:"code"`
	Message  string  `json:"message"`
	FuncName string  `json:"-"`
	FileName string  `json:"-"`
}

// New constructs an error based on an app error.
func New(code ErrCode, err error) *Error {
	return newError(code, err, 2)
}

func newError(code ErrCode, err error, callerSkip int) *Error {
	pc, filename, line, _ := runtime.Caller(callerSkip)

	return &Error{
		Code:     code,
		Message:  err.Error(),
		FuncName: runtime.FuncForPC(pc).Name(),
		FileName: fmt.Sprintf("%s:%d", filename, line),
	}
}

// FromKronk translates a Kronk SDK error into its HTTP application error.
//
// Every sentinel error defined across the SDK is listed here so that this
// function is the single place to audit the full error-mapping surface.
// Errors with a known, correct HTTP semantics are mapped to their ErrCode.
// Errors marked with a TODO comment are recognized but intentionally left
// at the default (Internal) code — they need discussion before a more
// specific code is assigned.
func FromKronk(err error) *Error {
	code := Internal
	switch {
	// -----------------------------------------------------------------
	// context errors — returned by the SDK when the caller's context
	// is canceled or the deadline is exceeded.
	// -----------------------------------------------------------------
	case errors.Is(err, context.Canceled):
		code = Canceled
	case errors.Is(err, context.DeadlineExceeded):
		code = DeadlineExceeded

	// -----------------------------------------------------------------
	// sdk/kronk/model — request validation
	// -----------------------------------------------------------------
	case errors.Is(err, model.ErrFileInputsUnsupported):
		code = InvalidArgument

	// -----------------------------------------------------------------
	// sdk/kronk/pool — model acquisition
	// -----------------------------------------------------------------
	case errors.Is(err, kronkpool.ErrServerBusy):
		code = Unavailable
	case errors.Is(err, kronkpool.ErrNoCapacity):
		code = ResourceExhausted

	// -----------------------------------------------------------------
	// sdk/kronk — admission control
	// -----------------------------------------------------------------
	case errors.Is(err, kronk.ErrAdmissionTimeout):
		code = ResourceExhausted

	// -----------------------------------------------------------------
	// sdk/pool/engine/resman — resource manager
	//
	// ErrNoCapacity is also surfaced via kronkpool.ErrNoCapacity above,
	// but is listed here so direct resman errors are caught as well.
	// -----------------------------------------------------------------
	case errors.Is(err, resman.ErrNoCapacity):
		code = ResourceExhausted
	case errors.Is(err, resman.ErrUnknownDevice):
		// TODO: Discuss — likely FailedPrecondition or Internal.
	case errors.Is(err, resman.ErrInvalidPlan):
		// TODO: Discuss — likely InvalidArgument or Internal.
	case errors.Is(err, resman.ErrDuplicateKey):
		// TODO: Discuss — likely AlreadyExists or Aborted.
	case errors.Is(err, resman.ErrNoGPUs):
		// TODO: Discuss — likely FailedPrecondition or Unavailable.

	// -----------------------------------------------------------------
	// sdk/kronk/hf — HuggingFace client
	//
	// These can surface during model resolution/download when the pool
	// tries to acquire a model that is not yet on disk.
	// -----------------------------------------------------------------
	case errors.Is(err, hf.ErrNotFound):
		code = NotFound
	case errors.Is(err, hf.ErrThrottled):
		code = TooManyRequests

	// -----------------------------------------------------------------
	// sdk/kronk/jsonrepair — LLM tool-call JSON repair
	//
	// Returned when the model produces JSON that cannot be repaired.
	// -----------------------------------------------------------------
	case errors.Is(err, jsonrepair.ErrIrrecoverable):
		// TODO: Discuss — likely InvalidArgument or Internal.

	// -----------------------------------------------------------------
	// sdk/bucky/pool — whisper model pool
	//
	// ErrServerBusy is the same sentinel as kronkpool.ErrServerBusy
	// (both alias engine.ErrServerBusy) but is listed here for
	// documentation completeness.
	// -----------------------------------------------------------------
	case errors.Is(err, buckypool.ErrServerBusy):
		code = Unavailable

	// -----------------------------------------------------------------
	// sdk/bucky/ffmpeg — audio transcoding dependency
	// -----------------------------------------------------------------
	case errors.Is(err, buckyffmpeg.ErrNotInstalled):
		// TODO: Discuss — likely FailedPrecondition or Unimplemented.

	// -----------------------------------------------------------------
	// sdk/tools/libs — llama library management
	// -----------------------------------------------------------------
	case errors.Is(err, libs.ErrReadOnly):
		// TODO: Discuss — likely FailedPrecondition.

	// -----------------------------------------------------------------
	// sdk/tools/bucky/libs — whisper library management
	// -----------------------------------------------------------------
	case errors.Is(err, buckylibs.ErrReadOnly):
		// TODO: Discuss — likely FailedPrecondition.

	// -----------------------------------------------------------------
	// sdk/tools/github — GitHub API client
	//
	// Can surface during library/model downloads.
	// -----------------------------------------------------------------
	case errors.Is(err, github.ErrRateLimited):
		code = TooManyRequests
	}

	return newError(code, err, 2)
}

// Errorf constructs an error based on a error message.
func Errorf(code ErrCode, format string, v ...any) *Error {
	pc, filename, line, _ := runtime.Caller(1)

	return &Error{
		Code:     code,
		Message:  fmt.Sprintf(format, v...),
		FuncName: runtime.FuncForPC(pc).Name(),
		FileName: fmt.Sprintf("%s:%d", filename, line),
	}
}

// Error implements the error interface.
func (e *Error) Error() string {
	return e.Message
}

// Encode implements the encoder interface.
func (e *Error) Encode() ([]byte, string, error) {
	data, err := json.Marshal(e)
	return data, "application/json", err
}

// HTTPStatus implements the web package httpStatus interface so the
// web framework can use the correct http status.
func (e *Error) HTTPStatus() int {
	return httpStatus[e.Code]
}

// Equal provides support for the go-cmp package and testing.
func (e *Error) Equal(e2 *Error) bool {
	return e.Code == e2.Code && e.Message == e2.Message
}

// =============================================================================

// FieldError is used to indicate an error with a specific request field.
type FieldError struct {
	Field string `json:"field"`
	Err   string `json:"error"`
}

// FieldErrors represents a collection of field errors.
type FieldErrors []FieldError

// NewFieldErrors creates a field errors.
func NewFieldErrors(field string, err error) *Error {
	fe := FieldErrors{
		{
			Field: field,
			Err:   err.Error(),
		},
	}

	return fe.ToError()
}

// Add adds a field error to the collection.
func (fe *FieldErrors) Add(field string, err error) {
	*fe = append(*fe, FieldError{
		Field: field,
		Err:   err.Error(),
	})
}

// ToError converts the field errors to an Error.
func (fe FieldErrors) ToError() *Error {
	return New(InvalidArgument, fe)
}

// Error implements the error interface.
func (fe FieldErrors) Error() string {
	d, err := json.Marshal(fe)
	if err != nil {
		return err.Error()
	}
	return string(d)
}
