package diagnose

type backendProbeState struct{ value uint8 }

var (
	backendProbeUnknown = backendProbeState{value: 0}
	backendProbeNone    = backendProbeState{value: 1}
	backendProbeDevices = backendProbeState{value: 2}
)
