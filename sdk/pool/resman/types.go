package resman

import "errors"

// Default values applied when a Config field is left at its zero value.
const (
	DefaultBudgetPercent = 80
	DefaultHeadroomBytes = 256 << 20 // 256 MiB per GPU safety margin.
)

// Sentinel errors returned by the manager.
var (
	// ErrNoCapacity is returned when a reservation would exceed the budget on
	// any required device or the system RAM budget.
	ErrNoCapacity = errors.New("resman: insufficient memory budget")

	// ErrUnknownDevice is returned when a plan references a device name the
	// manager does not know about.
	ErrUnknownDevice = errors.New("resman: unknown device")

	// ErrInvalidPlan is returned when a PlanRequest is malformed.
	ErrInvalidPlan = errors.New("resman: invalid plan request")

	// ErrDuplicateKey is returned when a reservation key is already in use.
	ErrDuplicateKey = errors.New("resman: key already reserved")

	// ErrNoGPUs is returned when a plan needs VRAM but the manager has no
	// GPU devices in its snapshot.
	ErrNoGPUs = errors.New("resman: no GPUs available")
)

// Device describes a single compute device known to the manager. Only GPU
// devices are tracked; CPU entries from a device snapshot are ignored.
type Device struct {
	Name       string // e.g. "CUDA0", "Metal".
	Type       string // "gpu_cuda", "gpu_metal", "gpu_rocm", "gpu_vulkan".
	TotalBytes int64
}

// Snapshot captures the system resources the manager will reason about. It is
// taken once when the manager is constructed.
//
// UnifiedMemory is true on systems where the GPU and CPU share a single
// physical memory pool (Apple Silicon Metal). When true, the manager tracks
// only the system-RAM pool and never adds a separate GPU device to its
// budget; this prevents double-counting the same physical bytes against two
// budgets. The flag is set by FromDevices when any GPU in the snapshot
// reports type "gpu_metal", and is forwarded to Usage so observability code
// can label the single pool appropriately.
type Snapshot struct {
	Devices       []Device
	RAMBytes      int64
	UnifiedMemory bool
}

// Config configures the resource manager.
//
// BudgetPercent is the fraction (1..100) of each device's total memory the
// manager will hand out. Defaults to DefaultBudgetPercent (80) when zero.
//
// HeadroomBytes is an additional per-GPU safety margin subtracted from the
// budget after applying BudgetPercent. Defaults to DefaultHeadroomBytes
// (256 MiB) when zero. Pass a negative number to opt out (clamped to zero).
type Config struct {
	Snapshot      Snapshot
	BudgetPercent int
	HeadroomBytes int64
}

// PlanRequest describes a model that wants to be loaded.
//
// VRAMBytes is the total predicted GPU memory the model + KV cache + compute
// buffer will consume. RAMBytes is the host-side memory it needs (typically
// zero today; reserved for a future CPU-offload feature).
//
// Devices, when non-empty, pins the request to specific device names. When
// combined with TensorSplit (same length, non-zero sum), VRAMBytes is split
// across the listed devices proportionally to the split values.
//
// When Devices is empty the manager picks the GPU with the most remaining
// budget that can fit VRAMBytes.
type PlanRequest struct {
	Key         string
	VRAMBytes   int64
	RAMBytes    int64
	Devices     []string
	TensorSplit []float32
}

// DeviceAllocation describes how many bytes were reserved on a specific GPU.
type DeviceAllocation struct {
	Index int    // Manager's internal device index.
	Name  string // Device name (e.g. "CUDA0").
	Bytes int64
}

// LoadPlan is the resolved plan returned by Reserve. It records exactly which
// devices were debited and by how much.
type LoadPlan struct {
	Key       string
	Per       []DeviceAllocation
	VRAMBytes int64
	RAMBytes  int64
}

// Ticket is returned by Reserve and is used to release the reservation.
type Ticket struct {
	Key string
}

// Usage describes the manager's current accounting, suitable for observability.
//
// UnifiedMemory mirrors the Snapshot flag: when true, the system uses a
// shared GPU/CPU memory pool (Apple Silicon Metal) and only the RAM
// budget/used fields are populated. The Devices slice will be empty.
type Usage struct {
	BudgetPercent int
	HeadroomBytes int64
	Devices       []DeviceUsage
	RAMTotal      int64
	RAMBudget     int64
	RAMUsed       int64
	Reservations  []LoadPlan
	UnifiedMemory bool
}

// DeviceUsage describes the accounting state for a single device.
type DeviceUsage struct {
	Index       int
	Name        string
	Type        string
	TotalBytes  int64
	BudgetBytes int64
	UsedBytes   int64
}
