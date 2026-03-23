// Package logging provides a flag to control whether logrus logging is enabled.
package logging

import "sync/atomic"

var enabled int32 = 1

func SetEnabled(v bool) {
	if v {
		atomic.StoreInt32(&enabled, 1)
	} else {
		atomic.StoreInt32(&enabled, 0)
	}
}

func Enabled() bool {
	return atomic.LoadInt32(&enabled) == 1
}
