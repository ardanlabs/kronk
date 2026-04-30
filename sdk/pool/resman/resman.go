// Package resman provides a resource manager that admits or rejects model
// loads based on a memory budget rather than a fixed model count. It tracks
// per-GPU VRAM reservations and a system RAM reservation, ensuring the sum
// of in-flight reservations never exceeds the configured budget.
//
// The manager is purely an in-memory accountant. It does not perform any I/O
// against the GPU or the OS; the byte counts it works with are supplied by
// callers (typically derived from models.CalculateVRAM and a devices.List
// snapshot taken once at startup).
package resman

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ardanlabs/kronk/sdk/tools/devices"
)

// Manager is the in-memory resource accountant.
type Manager struct {
	budgetPercent int
	headroomBytes int64
	devices       []Device
	deviceByName  map[string]int
	deviceBudget  []int64
	ramTotal      int64
	ramBudget     int64
	mu            sync.Mutex
	deviceUsed    []int64
	ramUsed       int64
	reservations  map[string]LoadPlan
}

// New constructs a manager from the provided snapshot and budget settings.
// CPU entries in the snapshot are ignored; only GPU devices are tracked.
func New(cfg Config) (*Manager, error) {
	if cfg.BudgetPercent == 0 {
		cfg.BudgetPercent = DefaultBudgetPercent
	}

	if cfg.BudgetPercent < 1 || cfg.BudgetPercent > 100 {
		return nil, fmt.Errorf("new: budget-percent[%d] must be between 1 and 100", cfg.BudgetPercent)
	}

	headroom := cfg.HeadroomBytes
	if headroom == 0 {
		headroom = DefaultHeadroomBytes
	}

	if headroom < 0 {
		headroom = 0
	}

	m := Manager{
		budgetPercent: cfg.BudgetPercent,
		headroomBytes: headroom,
		deviceByName:  make(map[string]int),
		ramTotal:      cfg.Snapshot.RAMBytes,
		reservations:  make(map[string]LoadPlan),
	}

	for _, d := range cfg.Snapshot.Devices {
		if !strings.HasPrefix(d.Type, "gpu_") {
			continue
		}

		if _, exists := m.deviceByName[d.Name]; exists {
			return nil, fmt.Errorf("new: duplicate device name[%s] in snapshot", d.Name)
		}

		idx := len(m.devices)
		m.deviceByName[d.Name] = idx
		m.devices = append(m.devices, d)

		budget := max(int64(float64(d.TotalBytes)*float64(cfg.BudgetPercent)/100.0)-headroom, 0)
		m.deviceBudget = append(m.deviceBudget, budget)
		m.deviceUsed = append(m.deviceUsed, 0)
	}

	if m.ramTotal > 0 {
		m.ramBudget = int64(float64(m.ramTotal) * float64(cfg.BudgetPercent) / 100.0)
	}

	return &m, nil
}

// FromDevices builds a Snapshot from a live devices.Devices result. The
// production wiring uses this to feed the manager from devices.List().
func FromDevices(d devices.Devices) Snapshot {
	out := Snapshot{
		RAMBytes: int64(d.SystemRAMBytes),
	}

	for _, di := range d.Devices {
		if !strings.HasPrefix(di.Type, "gpu_") {
			continue
		}

		out.Devices = append(out.Devices, Device{
			Name:       di.Name,
			Type:       di.Type,
			TotalBytes: int64(di.TotalBytes),
		})
	}

	return out
}

// HasGPUs reports whether the manager has any GPU devices in its snapshot.
func (m *Manager) HasGPUs() bool {
	return len(m.devices) > 0
}

// Reserve atomically plans and commits a reservation. On success it returns a
// ticket that must be passed to Release when the model is unloaded. On
// failure the manager state is unchanged.
func (m *Manager) Reserve(req PlanRequest) (Ticket, LoadPlan, error) {
	if req.Key == "" {
		return Ticket{}, LoadPlan{}, fmt.Errorf("reserve: key required: %w", ErrInvalidPlan)
	}
	if req.VRAMBytes < 0 || req.RAMBytes < 0 {
		return Ticket{}, LoadPlan{}, fmt.Errorf("reserve: bytes must be non-negative: %w", ErrInvalidPlan)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.reservations[req.Key]; exists {
		return Ticket{}, LoadPlan{}, fmt.Errorf("reserve: key[%s]: %w", req.Key, ErrDuplicateKey)
	}

	plan, err := m.planLocked(req)
	if err != nil {
		return Ticket{}, LoadPlan{}, err
	}

	if plan.RAMBytes > 0 && m.ramBudget > 0 && m.ramUsed+plan.RAMBytes > m.ramBudget {
		return Ticket{}, LoadPlan{}, fmt.Errorf("reserve: RAM[used=%d + need=%d > budget=%d]: %w",
			m.ramUsed, plan.RAMBytes, m.ramBudget, ErrNoCapacity)
	}

	for _, alloc := range plan.Per {
		m.deviceUsed[alloc.Index] += alloc.Bytes
	}

	m.ramUsed += plan.RAMBytes
	m.reservations[req.Key] = plan

	return Ticket{Key: req.Key}, plan, nil
}

// Release returns the ticket's reservation to the budget. Releasing an
// unknown ticket is a no-op.
func (m *Manager) Release(t Ticket) {
	if t.Key == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	plan, exists := m.reservations[t.Key]
	if !exists {
		return
	}

	for _, alloc := range plan.Per {
		m.deviceUsed[alloc.Index] -= alloc.Bytes
		if m.deviceUsed[alloc.Index] < 0 {
			m.deviceUsed[alloc.Index] = 0
		}
	}

	m.ramUsed -= plan.RAMBytes
	if m.ramUsed < 0 {
		m.ramUsed = 0
	}

	delete(m.reservations, t.Key)
}

// Usage returns a copy of the manager's current accounting.
func (m *Manager) Usage() Usage {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := Usage{
		BudgetPercent: m.budgetPercent,
		HeadroomBytes: m.headroomBytes,
		RAMBudget:     m.ramBudget,
		RAMUsed:       m.ramUsed,
		Devices:       make([]DeviceUsage, len(m.devices)),
		Reservations:  make([]LoadPlan, 0, len(m.reservations)),
	}

	for i, d := range m.devices {
		out.Devices[i] = DeviceUsage{
			Index:       i,
			Name:        d.Name,
			Type:        d.Type,
			TotalBytes:  d.TotalBytes,
			BudgetBytes: m.deviceBudget[i],
			UsedBytes:   m.deviceUsed[i],
		}
	}

	for _, p := range m.reservations {
		out.Reservations = append(out.Reservations, p)
	}

	return out
}

// =============================================================================

// planLocked resolves a PlanRequest into a LoadPlan against current
// reservations. The caller must hold m.mu.
func (m *Manager) planLocked(req PlanRequest) (LoadPlan, error) {
	plan := LoadPlan{
		Key:       req.Key,
		VRAMBytes: req.VRAMBytes,
		RAMBytes:  req.RAMBytes,
	}

	if req.VRAMBytes == 0 {
		return plan, nil
	}

	if len(m.devices) == 0 {
		return LoadPlan{}, fmt.Errorf("plan: VRAM[%d] requested: %w", req.VRAMBytes, ErrNoGPUs)
	}

	// Multi-device with explicit tensor split.
	if len(req.Devices) > 1 || (len(req.Devices) > 0 && len(req.TensorSplit) > 0) {
		return m.planSplitLocked(req, plan)
	}

	// Single pinned device by name.
	if len(req.Devices) == 1 {
		idx, ok := m.deviceByName[req.Devices[0]]
		if !ok {
			return LoadPlan{}, fmt.Errorf("plan: device[%s]: %w", req.Devices[0], ErrUnknownDevice)
		}

		if !m.fitsLocked(idx, req.VRAMBytes) {
			return LoadPlan{}, m.noCapacityErr(req.Devices[0], idx, req.VRAMBytes)
		}

		plan.Per = []DeviceAllocation{{Index: idx, Name: m.devices[idx].Name, Bytes: req.VRAMBytes}}

		return plan, nil
	}

	// Free choice: best-fit by remaining headroom.
	bestIdx := -1
	var bestRoom int64 = -1
	for idx := range m.devices {
		room := m.deviceBudget[idx] - m.deviceUsed[idx]

		if room < req.VRAMBytes {
			continue
		}

		if room > bestRoom {
			bestRoom = room
			bestIdx = idx
		}
	}

	if bestIdx < 0 {
		return LoadPlan{}, fmt.Errorf("plan: no GPU has %d bytes free across %d device(s): %w",
			req.VRAMBytes, len(m.devices), ErrNoCapacity)
	}

	plan.Per = []DeviceAllocation{{
		Index: bestIdx,
		Name:  m.devices[bestIdx].Name,
		Bytes: req.VRAMBytes,
	}}

	return plan, nil
}

// planSplitLocked handles the multi-device case. When TensorSplit is supplied
// it is used as the proportion vector; otherwise the request is split
// proportional to each listed device's TotalBytes (matching llama.cpp's
// auto-split heuristic).
func (m *Manager) planSplitLocked(req PlanRequest, plan LoadPlan) (LoadPlan, error) {
	if len(req.TensorSplit) > 0 && len(req.TensorSplit) != len(req.Devices) {
		return LoadPlan{}, fmt.Errorf("plan: devices[%d] != tensor-split[%d]: %w",
			len(req.Devices), len(req.TensorSplit), ErrInvalidPlan)
	}

	weights := make([]float64, len(req.Devices))
	var sum float64

	switch {
	case len(req.TensorSplit) > 0:
		for i, s := range req.TensorSplit {
			if s < 0 {
				return LoadPlan{}, fmt.Errorf("plan: negative tensor-split[%d]: %w", i, ErrInvalidPlan)
			}

			weights[i] = float64(s)
			sum += weights[i]
		}

	default:
		for i, name := range req.Devices {
			idx, ok := m.deviceByName[name]
			if !ok {
				return LoadPlan{}, fmt.Errorf("plan: device[%s]: %w", name, ErrUnknownDevice)
			}

			weights[i] = float64(m.devices[idx].TotalBytes)
			sum += weights[i]
		}
	}

	if sum <= 0 {
		return LoadPlan{}, fmt.Errorf("plan: tensor-split sum is zero: %w", ErrInvalidPlan)
	}

	plan.Per = make([]DeviceAllocation, 0, len(req.Devices))
	var assigned int64

	for i, name := range req.Devices {
		idx, ok := m.deviceByName[name]
		if !ok {
			return LoadPlan{}, fmt.Errorf("plan: device[%s]: %w", name, ErrUnknownDevice)
		}

		var bytes int64
		if i == len(req.Devices)-1 {
			// Assign the remainder to the last device so rounding never causes
			// the sum of allocations to drift below req.VRAMBytes.
			bytes = req.VRAMBytes - assigned
		} else {
			bytes = int64(float64(req.VRAMBytes) * weights[i] / sum)
			assigned += bytes
		}

		if !m.fitsLocked(idx, bytes) {
			return LoadPlan{}, m.noCapacityErr(name, idx, bytes)
		}

		plan.Per = append(plan.Per, DeviceAllocation{Index: idx, Name: m.devices[idx].Name, Bytes: bytes})
	}

	return plan, nil
}

// fitsLocked reports whether the device at idx can absorb an additional
// `need` bytes without exceeding its budget. Caller must hold m.mu.
func (m *Manager) fitsLocked(idx int, need int64) bool {
	return m.deviceUsed[idx]+need <= m.deviceBudget[idx]
}

// noCapacityErr produces a descriptive ErrNoCapacity error for a specific
// device. Caller must hold m.mu.
func (m *Manager) noCapacityErr(name string, idx int, need int64) error {
	return fmt.Errorf("plan: device[%s] cannot fit need=%d bytes (used=%d budget=%d): %w",
		name, need, m.deviceUsed[idx], m.deviceBudget[idx], ErrNoCapacity)
}
