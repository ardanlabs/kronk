//go:build windows

package diagnose

import (
	"strconv"
	"strings"
)

// parseSystem extracts the CPU model and total RAM (bytes) from the captured
// Windows commands. CPU comes from the "Name :" line of Win32_Processor; RAM
// comes from the "TotalPhysicalMemory :" line of Win32_ComputerSystem, which
// reports bytes. Both are Format-List output of the form "Key : Value".
func parseSystem(cmds []Command) (cpuModel string, ramBytes uint64) {
	cpuModel = formatListValue(commandOutput(cmds, "Win32_Processor"), "Name")
	ramBytes, _ = strconv.ParseUint(formatListValue(commandOutput(cmds, "Win32_ComputerSystem"), "TotalPhysicalMemory"), 10, 64)
	return cpuModel, ramBytes
}

// formatListValue returns the value for key from PowerShell Format-List output,
// whose lines look like "Key                 : Value".
func formatListValue(out, key string) string {
	for line := range strings.SplitSeq(out, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(name) == key {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// systemCommandSpecs returns the host/device commands to capture on Windows via
// systeminfo and PowerShell CIM queries.
func systemCommandSpecs() []commandSpec {
	ps := func(expr string) commandSpec {
		return commandSpec{"powershell", []string{"-NoProfile", "-Command", expr}}
	}

	return []commandSpec{
		// OS + CPU + memory overview.
		{"systeminfo", nil},

		// Structured CPU / OS / RAM / GPU via CIM (PowerShell).
		ps("Get-CimInstance Win32_Processor | Format-List Name,NumberOfCores,NumberOfLogicalProcessors"),
		ps("Get-CimInstance Win32_OperatingSystem | Format-List Caption,Version,BuildNumber,TotalVisibleMemorySize,FreePhysicalMemory"),
		ps("Get-CimInstance Win32_ComputerSystem | Format-List Manufacturer,Model,TotalPhysicalMemory"),
		ps("Get-CimInstance Win32_VideoController | Format-List Name,AdapterRAM,DriverVersion"),

		// GPU cards (NVIDIA), if drivers/tooling are present.
		{"nvidia-smi", nil},
	}
}
