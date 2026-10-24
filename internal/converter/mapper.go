package converter

import (
	"reflect"
	"strings"
	"time"
)

// 本包把 core 层实体搬成 thrift 生成的对外 DTO。搬运规则集中在这里,
// 各转换函数只声明"例外"(见 fieldSpec),不再逐字段抄写。
//
// 机制要点:
//   - 字段按"归一化名"匹配(core 的 InstanceURL 与 DTO 的 InstanceUrl 归一化后同名),
//     所以 core 加字段时通常只改 core 与 thrift 两处,转换层不用动。
//   - 类型不兼容时保持 DTO 零值(不 panic),由 mapper_test.go 的覆盖度测试兜住。
//   - 没有代码生成、没有新依赖,只有 reflect + 标准库。

// 时间格式:DTO 里的时间一律是 string,格式按家族区分。
const (
	layoutDateTime = "2006-01-02 15:04:05" // 任务/运行/步骤/webhook/审计日志
	layoutRFC3339  = time.RFC3339          // 平台/仓库(前端 new Date() 能可靠解析)
)

// fieldSpec 描述「一个 core 实体家族 → 一个 DTO 家族」的字段搬运契约。
//
// 契约只声明例外,不声明正例:DTO 字段默认按归一化字段名自动匹配 core 字段。
// 因此 core 实体加字段、thrift DTO 同步加字段之后,转换层通常无需修改。
type fieldSpec struct {
	// layout:time.Time / *time.Time → string 的时间格式。
	layout string
	// aliases:DTO 字段名(归一化) → core 字段名(归一化)。
	// 仅用于两边概念同名、拼写不同构的情况(如 DTO.Time ← core.CreatedAt)。
	aliases map[string]string
	// skip:明确不搬运的 DTO 字段(归一化名)。
	// 用于 secret 字段,以及列表视图刻意省略的字段。
	skip map[string]bool
	// derived:由派生逻辑填充、而非从 core 字段搬运的 DTO 字段(归一化名)。
	// 这些字段的取值是算出来的(如 HasToken = AccessToken != ""),不是搬来的。
	derived map[string]bool
}

// mapFields 把 src(core 实体)的字段搬到 dst(thrift DTO)上。
//
// 匹配顺序:归一化后同名 → spec.aliases → 放弃(保持 DTO 零值)。
// spec.skip 命中的字段一律不搬。
func mapFields(dst, src any, spec fieldSpec) {
	dv := reflect.ValueOf(dst)
	if dv.Kind() != reflect.Pointer || dv.IsNil() {
		return
	}
	dv = dv.Elem()
	if dv.Kind() != reflect.Struct {
		return
	}
	sv := reflect.ValueOf(src)
	for sv.Kind() == reflect.Pointer {
		if sv.IsNil() {
			return
		}
		sv = sv.Elem()
	}
	if sv.Kind() != reflect.Struct {
		return
	}

	srcIdx := indexFields(sv.Type())
	dstType := dv.Type()
	for i := 0; i < dstType.NumField(); i++ {
		df := dstType.Field(i)
		if df.PkgPath != "" { // 未导出字段
			continue
		}
		name := normalizeField(df.Name)
		if spec.skip[name] {
			continue
		}
		si, ok := srcIdx[name]
		if !ok {
			if alias, has := spec.aliases[name]; has {
				si, ok = srcIdx[alias]
			}
			if !ok {
				continue
			}
		}
		assignValue(dv.Field(i), sv.Field(si), spec)
	}
}

// indexFields 建立「归一化字段名 → 字段下标」索引;同名时先出现的赢。
func indexFields(t reflect.Type) map[string]int {
	idx := make(map[string]int, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		n := normalizeField(f.Name)
		if _, exists := idx[n]; !exists {
			idx[n] = i
		}
	}
	return idx
}

// normalizeField 归一化 Go 字段名,让"同一概念的不同拼写"能对上。
//
//	InstanceURL → instanceurl      InstanceUrl → instanceurl
//	GitLFS     → gitlfs            GitLfs     → gitlfs
//	CommitSHA  → commitsha         CommitSha  → commitsha
//	WebhookEventID → webhookeventid WebhookEventId → webhookeventid
//
// 规则:去掉 '_' 后转小写。core 与 thrift 的缩写差异恰好只差大小写
// (URL/Url、LFS/Lfs、SHA/Sha、TLS/Tls),归一化后一致。
func normalizeField(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", ""))
}

// assignValue 把一个 core 字段的值搬进一个 DTO 字段。
// 类型对不上时保持 DTO 零值——覆盖度测试会把它抓出来。
func assignValue(dst, src reflect.Value, spec fieldSpec) {
	if src.Kind() == reflect.Pointer {
		if src.IsNil() {
			return // nil → DTO 保持零值(nil 指针 / 空串)
		}
		if dst.Kind() == reflect.Pointer {
			elem := reflect.New(dst.Type().Elem())
			assignValue(elem.Elem(), src.Elem(), spec)
			dst.Set(elem)
			return
		}
		assignValue(dst, src.Elem(), spec)
		return
	}
	if dst.Kind() == reflect.Pointer {
		return // 值 → 指针:语义不明确,不猜,保持零值
	}
	if src.Type() == dst.Type() {
		dst.Set(src)
		return
	}
	if src.Type() == reflect.TypeOf(time.Time{}) && dst.Kind() == reflect.String {
		dst.SetString(src.Interface().(time.Time).Format(spec.layout))
		return
	}
	if isNumericKind(src.Kind()) && isNumericKind(dst.Kind()) {
		assignNumeric(dst, src)
		return
	}
	if src.Kind() == reflect.Slice && dst.Kind() == reflect.Slice {
		assignSlice(dst, src, spec)
	}
}

// assignSlice 搬 []struct → []*struct(运行记录的 Steps 走这条路)。
// 空切片保持 DTO 为 nil,JSON 仍是 null。
func assignSlice(dst, src reflect.Value, spec fieldSpec) {
	if src.Len() == 0 {
		return
	}
	de := dst.Type().Elem()
	se := src.Type().Elem()
	if de.Kind() != reflect.Pointer || de.Elem().Kind() != reflect.Struct || se.Kind() != reflect.Struct {
		return
	}
	out := reflect.MakeSlice(dst.Type(), src.Len(), src.Len())
	for i := 0; i < src.Len(); i++ {
		elem := reflect.New(de.Elem())
		mapFields(elem.Interface(), src.Index(i).Interface(), spec)
		out.Index(i).Set(elem)
	}
	dst.Set(out)
}

// assignNumeric 搬数值字段,窄化时按区间钳制(与 safe_convert.go 的 Safe* 同口径):
// int→int32 钳 [MinInt32,MaxInt32];int64→int32 同;uint→int32 钳 [0,MaxInt32];
// uint→int64 直转(与 SafeUintToInt64 一致)。
func assignNumeric(dst, src reflect.Value) {
	switch src.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v := src.Int()
		if isUnsignedKind(dst.Kind()) {
			if v < 0 {
				return
			}
			dst.SetUint(clampUint64(uint64(v), dst.Type().Bits()))
			return
		}
		dst.SetInt(clampInt64(v, dst.Type().Bits()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v := src.Uint()
		if isUnsignedKind(dst.Kind()) {
			dst.SetUint(clampUint64(v, dst.Type().Bits()))
			return
		}
		if dst.Type().Bits() >= 64 {
			dst.SetInt(int64(v)) // 与 SafeUintToInt64 同语义
			return
		}
		max := uint64(1)<<(dst.Type().Bits()-1) - 1
		if v > max {
			dst.SetInt(int64(max))
			return
		}
		dst.SetInt(int64(v))
	}
}

func clampInt64(v int64, bits int) int64 {
	if bits >= 64 {
		return v
	}
	min := -(int64(1) << (bits - 1))
	max := int64(1)<<(bits-1) - 1
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func clampUint64(v uint64, bits int) uint64 {
	if bits >= 64 {
		return v
	}
	max := uint64(1)<<bits - 1
	if v > max {
		return max
	}
	return v
}

func isNumericKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func isUnsignedKind(k reflect.Kind) bool {
	switch k {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}
