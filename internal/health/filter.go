package health

import (
	"path"
	"strings"
)

// Filter 过滤链(借鉴 gickup/ghorg):
// Include 优先(非空时只留命中);Exclude 再剔除。
// IncludeGlobs/ExcludeGlobs 支持 * 通配;匹配对象是 key 或 name。
type Filter struct {
	Include      []string `yaml:"include" json:"include"`
	Exclude      []string `yaml:"exclude" json:"exclude"`
	IncludeGlobs []string `yaml:"include_globs" json:"include_globs"`
	ExcludeGlobs []string `yaml:"exclude_globs" json:"exclude_globs"`
}

// Allow 判断 key/name 是否通过过滤。
func (f *Filter) Allow(key, name string) bool {
	if f == nil {
		return true
	}
	if len(f.Include) == 0 && len(f.IncludeGlobs) == 0 {
		// 无 include 则默认全过,再走 exclude
		return !f.excluded(key, name)
	}
	included := f.included(key, name)
	if !included {
		return false
	}
	return !f.excluded(key, name)
}

func (f *Filter) included(key, name string) bool {
	for _, x := range f.Include {
		if x == key || x == name {
			return true
		}
	}
	for _, g := range f.IncludeGlobs {
		if globMatch(g, key) || globMatch(g, name) {
			return true
		}
	}
	return false
}

func (f *Filter) excluded(key, name string) bool {
	for _, x := range f.Exclude {
		if x == key || x == name {
			return true
		}
	}
	for _, g := range f.ExcludeGlobs {
		if globMatch(g, key) || globMatch(g, name) {
			return true
		}
	}
	return false
}

func globMatch(pattern, s string) bool {
	if ok, err := path.Match(pattern, s); err == nil && ok {
		return true
	}
	// 也允许子串风格的 *foo*
	return strings.Contains(s, strings.Trim(pattern, "*")) && strings.Contains(pattern, "*")
}
