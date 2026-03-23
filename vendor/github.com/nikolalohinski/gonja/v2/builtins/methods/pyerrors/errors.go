// Package pyerrors provides Python-compatible error types.
package pyerrors

import "fmt"

var (
	ErrValue     = fmt.Errorf("ValueError")
	ErrIndex     = fmt.Errorf("IndexError")
	ErrInternal  = fmt.Errorf("InternalError")
	ErrKey       = fmt.Errorf("KeyError")
	ErrArguments = fmt.Errorf("ArgumentError")
	ErrOverflow  = fmt.Errorf("OverflowError")
)
