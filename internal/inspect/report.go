package inspect

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// CPUSample holds the previous process times reading for a coarse CPU estimate.
type CPUSample struct {
	At        time.Time
	UserSec   float64
	SystemSec float64
}

const maxTreeLines = 7
const maxListenPorts = 8

// Build returns human-readable inspector lines for an OS pid.
// Pass prev from the prior refresh for the same pid to refine CPU; otherwise CPU may be omitted.
func Build(pid int, prev *CPUSample) (lines []string, next *CPUSample, notes []string) {
	if pid <= 0 {
		return []string{"(invalid pid)"}, nil, nil
	}

	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return []string{fmt.Sprintf("Cannot open process %d: %v", pid, err)}, nil, nil
	}

	name, _ := p.Name()
	cmdStr, _ := p.Cmdline()
	if cmdStr == "" {
		cmdStr = "(unknown)"
	}
	if len(cmdStr) > 120 {
		cmdStr = cmdStr[:117] + "…"
	}

	var rss uint64
	if mi, err := p.MemoryInfo(); err == nil && mi != nil {
		rss = mi.RSS
	}

	nth, _ := p.NumThreads()
	cpuLine := ""
	times, err := p.Times()
	if err == nil && times != nil {
		next = &CPUSample{At: time.Now(), UserSec: times.User, SystemSec: times.System}
		if prev != nil && !prev.At.IsZero() {
			dt := next.At.Sub(prev.At).Seconds()
			if dt >= 0.25 {
				du := times.User - prev.UserSec
				ds := times.System - prev.SystemSec
				if dt > 0 {
					cpuLine = fmt.Sprintf("CPU ~%.0f%% (since last refresh)", (du+ds)/dt*100)
				}
			}
		}
	}
	if cpuLine == "" {
		cpuLine = "CPU: … (refreshes while inspector is open)"
	}

	lines = append(lines,
		fmt.Sprintf("pid %d · %s", pid, name),
		cmdStr,
		fmt.Sprintf("RSS ~%s · threads %d", formatBytes(rss), nth),
		cpuLine,
		"",
		"Direct child processes:",
	)
	lines = append(lines, directChildrenLines(p)...)

	lines = append(lines, "", "Listening ports (this pid):")
	ports, portNotes := listeningPorts(int32(pid))
	lines = append(lines, ports...)
	notes = append(notes, portNotes...)

	if runtime.GOOS == "windows" {
		notes = append(notes, "Some metrics are limited on Windows compared to POSIX.")
	}
	if runtime.GOOS == "darwin" {
		notes = append(notes, "Port listing may be incomplete without elevated permissions.")
	}

	return lines, next, notes
}

func formatBytes(n uint64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KiB", "MiB", "GiB"}
	v := float64(n)
	u := 0
	for v >= 1024 && u < len(units)-1 {
		v /= 1024
		u++
	}
	return fmt.Sprintf("%.1f %s", v, units[u])
}

func directChildrenLines(p *process.Process) []string {
	children, err := p.Children()
	if err != nil || len(children) == 0 {
		return []string{"  (none)"}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Pid < children[j].Pid })
	var out []string
	for _, c := range children {
		if len(out) >= maxTreeLines {
			out = append(out, "  …")
			break
		}
		nm, _ := c.Name()
		out = append(out, fmt.Sprintf("  %d %s", c.Pid, nm))
	}
	return out
}

func listeningPorts(pid int32) ([]string, []string) {
	var notes []string
	conns, err := net.Connections("inet")
	if err != nil {
		return []string{fmt.Sprintf("  (unavailable: %v)", err)}, []string{fmt.Sprintf("net.Connections: %v", err)}
	}
	var hits []uint32
	seen := map[uint32]struct{}{}
	for _, c := range conns {
		if c.Pid != pid {
			continue
		}
		if !strings.EqualFold(c.Status, "LISTEN") {
			continue
		}
		port := c.Laddr.Port
		if _, dup := seen[port]; dup {
			continue
		}
		seen[port] = struct{}{}
		hits = append(hits, port)
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i] < hits[j] })
	if len(hits) == 0 {
		return []string{"  (none found)"}, notes
	}
	if len(hits) > maxListenPorts {
		notes = append(notes, fmt.Sprintf("Showing %d of %d listening sockets.", maxListenPorts, len(hits)))
		hits = hits[:maxListenPorts]
	}
	lines := make([]string, 0, len(hits))
	for _, port := range hits {
		lines = append(lines, fmt.Sprintf("  %d", port))
	}
	return lines, notes
}
