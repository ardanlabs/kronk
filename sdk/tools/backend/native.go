package backend

import "sync"

// NativeInit serializes Kronk, Bucky, and Malina initialization so their GGML
// libraries cannot load concurrently. Hold its lock throughout initialization and when
// accessing the load flags. Flags remain set after later initialization errors:
// successfully loaded native libraries are still resident in the process.
var NativeInit struct {
	sync.Mutex
	LlamaLoaded   bool
	WhisperLoaded bool
	SDLoaded      bool
}
