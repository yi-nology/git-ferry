package git_sync

// optStr optional string → 值(空串表示未填)。
func optStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// optBool optional bool → 值(缺省 false)。
func optBool(p *bool) bool {
	return p != nil && *p
}

// optBoolDefault optional bool,默认 true(适配 with_* 默认全开)。
func optBoolDefault(p *bool) bool {
	return p == nil || *p
}
