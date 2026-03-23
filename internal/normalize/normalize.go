// Package normalize provides utilities for transforming RouterOS protocol responses
// into more usable Go data structures with proper type conversions.
package normalize

import (
	"strconv"
	"strings"

	"github.com/go-routeros/routeros/v3/proto"
)

// Normalize converts RouterOS protocol sentences into a slice of maps
// with type-appropriate conversions. It renames ".id" to "id" and converts
// numeric fields (cpu-load, rx-bps, etc.) and uptime to proper numeric types.
func Normalize(re []*proto.Sentence) []map[string]any {
	out := make([]map[string]any, 0, len(re))
	for _, row := range re {
		m := map[string]any{}
		for k, v := range row.Map {
			if k == ".id" {
				k = "id"
			}
			switch k {
			case "cpu-load", "rx-bps", "tx-bps", "bytes-in", "bytes-out",
				"free-memory", "total-memory", "free-hdd-space", "total-hdd-space":
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					m[k] = n
					continue
				}
			case "uptime":
				m[k] = UptimeToSeconds(v)
				continue
			}
			m[k] = v
		}
		out = append(out, m)
	}
	return out
}

// UptimeToSeconds converts RouterOS uptime strings to seconds.
// Supports formats like "1w2d3h4m5s", "2d12:30:45", or "12:30:45".
// Returns the total duration in seconds.
func UptimeToSeconds(s string) int64 {
	if strings.Contains(s, ":") {
		var days int64
		if strings.Contains(s, "d") {
			parts := strings.SplitN(s, "d", 2)
			days, _ = strconv.ParseInt(parts[0], 10, 64)
			s = parts[1]
		}

		var hms int64
		seg := strings.Split(s, ":")
		for _, n := range seg {
			v, _ := strconv.ParseInt(n, 10, 64)
			hms = hms*60 + v
		}
		return (days * 24 * 3600) + hms
	}

	var duration int64
	var currentVal int64
	for _, char := range s {
		if char >= '0' && char <= '9' {
			currentVal = currentVal*10 + int64(char-'0')
		} else {
			switch char {
			case 'w':
				duration += currentVal * 7 * 24 * 3600
			case 'd':
				duration += currentVal * 24 * 3600
			case 'h':
				duration += currentVal * 3600
			case 'm':
				duration += currentVal * 60
			case 's':
				duration += currentVal
			}
			currentVal = 0
		}
	}
	if currentVal > 0 {
		duration += currentVal
	}
	return duration
}

// EnsureAPIAddr ensures the host string includes the RouterOS API port.
// If no port is specified, it appends the default port ":8728".
func EnsureAPIAddr(host string) string {
	if strings.Contains(host, ":") {
		return host
	}
	return host + ":8728"
}
