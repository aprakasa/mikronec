package normalize

import (
	"testing"

	"github.com/go-routeros/routeros/v3/proto"
)

func TestUptimeToSeconds(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"simple hours", "01:00:00", 3600},
		{"hours minutes seconds", "01:30:45", 5400 + 45},
		{"days and time", "2d01:00:00", 2*24*3600 + 3600},
		{"weeks", "1w", 7 * 24 * 3600},
		{"days", "3d", 3 * 24 * 3600},
		{"hours", "5h", 5 * 3600},
		{"minutes", "30m", 30 * 60},
		{"seconds", "45s", 45},
		{"mixed", "1w2d3h4m5s", 1*7*24*3600 + 2*24*3600 + 3*3600 + 4*60 + 5},
		{"just number", "100", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UptimeToSeconds(tt.input)
			if result != tt.expected {
				t.Errorf("UptimeToSeconds(%q) = %d; want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEnsureAPIAddr(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{"with port", "192.168.1.1:8728", "192.168.1.1:8728"},
		{"without port", "192.168.1.1", "192.168.1.1:8728"},
		{"ipv6 with port", "[::1]:8728", "[::1]:8728"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnsureAPIAddr(tt.host)
			if result != tt.expected {
				t.Errorf("EnsureAPIAddr(%q) = %q; want %q", tt.host, result, tt.expected)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	sentences := []*proto.Sentence{
		{
			Map: map[string]string{
				".id":         "1",
				"name":        "test",
				"cpu-load":    "10",
				"uptime":      "1h",
				"free-memory": "1000",
			},
		},
	}

	result := Normalize(sentences)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	r := result[0]

	if r["id"] != "1" {
		t.Errorf("expected id to be '1', got %v", r["id"])
	}

	if r["name"] != "test" {
		t.Errorf("expected name to be 'test', got %v", r["name"])
	}

	cpuLoad, ok := r["cpu-load"].(int64)
	if !ok || cpuLoad != 10 {
		t.Errorf("expected cpu-load to be int64 10, got %v", r["cpu-load"])
	}

	uptime, ok := r["uptime"].(int64)
	if !ok || uptime != 3600 {
		t.Errorf("expected uptime to be 3600, got %v", r["uptime"])
	}

	freeMem, ok := r["free-memory"].(int64)
	if !ok || freeMem != 1000 {
		t.Errorf("expected free-memory to be 1000, got %v", r["free-memory"])
	}
}

func TestNormalizeKeepsNonNumeric(t *testing.T) {
	sentences := []*proto.Sentence{
		{
			Map: map[string]string{
				"name": "router1",
			},
		},
	}

	result := Normalize(sentences)

	if result[0]["name"] != "router1" {
		t.Errorf("expected name to be 'router1', got %v", result[0]["name"])
	}
}
