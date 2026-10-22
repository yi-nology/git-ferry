package converter

import (
	"math"

	"github.com/oekit/pkg/mathx"
)

// SafeUintToInt64 safely converts uint to int64.
// On 64-bit systems, uint is 64 bits, so this is always safe.
// On 32-bit systems, uint is 32 bits, which fits in int64.
func SafeUintToInt64(v uint) int64 { //nolint:gosec // uint to int64 is safe on all platforms
	return int64(v)
}

// SafeIntToInt32 safely converts int to int32 with bounds checking.
// 用 oekit mathx.Clamp 做区间钳制——与手写的两次比较逐字同构。
func SafeIntToInt32(v int) int32 {
	return int32(mathx.Clamp(v, math.MinInt32, math.MaxInt32))
}

// SafeInt64ToInt32 safely converts int64 to int32 with bounds checking.
func SafeInt64ToInt32(v int64) int32 {
	return int32(mathx.Clamp(v, math.MinInt32, math.MaxInt32))
}

// SafeUintToInt32 safely converts uint to int32 with bounds checking.
// uint 无下界,只需钳上界(与手写实现一致:不额外加下界判断)。
func SafeUintToInt32(v uint) int32 {
	return int32(mathx.Clamp(v, 0, math.MaxInt32))
}
