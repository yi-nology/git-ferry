package converter

import "math"

// SafeUintToInt64 safely converts uint to int64.
// On 64-bit systems, uint is 64 bits, so this is always safe.
// On 32-bit systems, uint is 32 bits, which fits in int64.
func SafeUintToInt64(v uint) int64 { //nolint:gosec // uint to int64 is safe on all platforms
	return int64(v)
}

// SafeIntToInt32 safely converts int to int32 with bounds checking.
func SafeIntToInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

// SafeInt64ToInt32 safely converts int64 to int32 with bounds checking.
func SafeInt64ToInt32(v int64) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

// SafeUintToInt32 safely converts uint to int32 with bounds checking.
func SafeUintToInt32(v uint) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(v)
}
