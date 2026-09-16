package converter

import (
	"math"
	"testing"
)

func TestSafeUintToInt64(t *testing.T) {
	tests := []struct {
		name     string
		input    uint
		expected int64
	}{
		{"zero", 0, 0},
		{"positive", 42, 42},
		{"max_uint32", math.MaxUint32, int64(math.MaxUint32)},
		{"large_value", math.MaxUint64, -1}, // overflow on 64-bit
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SafeUintToInt64(tt.input)
			if result != tt.expected {
				t.Errorf("SafeUintToInt64(%d) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSafeIntToInt32(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int32
	}{
		{"zero", 0, 0},
		{"positive", 42, 42},
		{"negative", -42, -42},
		{"max_int32", math.MaxInt32, math.MaxInt32},
		{"min_int32", math.MinInt32, math.MinInt32},
		{"overflow_max", math.MaxInt32 + 1, math.MaxInt32},
		{"overflow_min", math.MinInt32 - 1, math.MinInt32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SafeIntToInt32(tt.input)
			if result != tt.expected {
				t.Errorf("SafeIntToInt32(%d) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSafeInt64ToInt32(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected int32
	}{
		{"zero", 0, 0},
		{"positive", 42, 42},
		{"negative", -42, -42},
		{"max_int32", math.MaxInt32, math.MaxInt32},
		{"min_int32", math.MinInt32, math.MinInt32},
		{"overflow_max", math.MaxInt32 + 1, math.MaxInt32},
		{"overflow_min", math.MinInt32 - 1, math.MinInt32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SafeInt64ToInt32(tt.input)
			if result != tt.expected {
				t.Errorf("SafeInt64ToInt32(%d) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSafeUintToInt32(t *testing.T) {
	tests := []struct {
		name     string
		input    uint
		expected int32
	}{
		{"zero", 0, 0},
		{"positive", 42, 42},
		{"max_int32", math.MaxInt32, math.MaxInt32},
		{"overflow", math.MaxInt32 + 1, math.MaxInt32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SafeUintToInt32(tt.input)
			if result != tt.expected {
				t.Errorf("SafeUintToInt32(%d) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}
