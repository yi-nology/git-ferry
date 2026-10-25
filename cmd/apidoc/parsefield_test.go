package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseField_OptionalKeyword thrift 的 optional 关键字必须被正确处理。
//
// 回归背景(契约在说谎):解析器没处理 optional ⇒ "optional bool enabled" 被解析成
// typ="optional"、name="bool",类型落到未知分支 ⇒ **OpenAPI 里所有 optional
// 字段都成了 "type":"string"**(enabled / git_force / git_lfs / git_bundle /
// sync_wiki / submodules / git_push_prune …)。
// ⇒ 按已发布契约生成的客户端会把布尔字段发成字符串。
//
// 这条用例是**反向验证**:去掉 optional 的处理,本用例变红。
func TestParseField_OptionalKeyword(t *testing.T) {
	cases := []struct {
		line string
		typ  string
		name string
	}{
		{line: `8: optional bool enabled (api.json="enabled")`, typ: "bool", name: "enabled"},
		{line: `7: optional i32 minInterval (api.json="min_interval")`, typ: "i32", name: "minInterval"},
		{line: `6: optional string cron (api.json="cron")`, typ: "string", name: "cron"},
		{line: `2: string name (api.json="name")`, typ: "string", name: "name"},
		{line: `12: bool enabled (api.json="enabled")`, typ: "bool", name: "enabled"},
	}
	for _, c := range cases {
		f := parseField(c.line)
		require.NotNil(t, f, "line=%s", c.line)
		assert.Equal(t, c.typ, f.Type, "类型解析错误: line=%s", c.line)
		assert.Equal(t, c.name, f.Name, "字段名解析错误: line=%s", c.line)
	}
}

// TestParseField_OptionalKeepsAnnotations optional 字段的注解不能丢。
func TestParseField_OptionalKeepsAnnotations(t *testing.T) {
	f := parseField(`8: optional bool enabled (api.json="enabled")`)
	require.NotNil(t, f)
	assert.Equal(t, "enabled", f.JSONName)
}
