package malina

// ProgressFunc receives process-global model loading and image generation
// progress. SecondsPerStep is the average elapsed time per completed step. The
// function can be called concurrently and must be concurrency-safe.
type ProgressFunc func(step int, steps int, secondsPerStep float32)

// DiscardProgress discards model loading and image generation progress.
var DiscardProgress ProgressFunc = func(step int, steps int, secondsPerStep float32) {}
