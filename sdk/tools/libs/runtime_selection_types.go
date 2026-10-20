package libs

type runtimeProbeState struct{ value uint8 }

var (
	runtimeProbeUnknown = runtimeProbeState{value: 0}
	runtimeProbeNone    = runtimeProbeState{value: 1}
	runtimeProbeDevices = runtimeProbeState{value: 2}
)

type hostCUDAState struct{ value uint8 }

var (
	hostCUDAUnknown     = hostCUDAState{value: 0}
	hostCUDASupported   = hostCUDAState{value: 1}
	hostCUDAUnsupported = hostCUDAState{value: 2}
	hostCUDAUnavailable = hostCUDAState{value: 3}
)
