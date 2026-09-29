package textutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizePathToken(t *testing.T) {
	assert.Equal(t, "a_b", SanitizePathToken("a/b"))
	assert.Equal(t, "a_b", SanitizePathToken(`a\b`))
	assert.Equal(t, "_", SanitizePathToken(".."))
	assert.Equal(t, "unnamed", SanitizePathToken(""))
	assert.Equal(t, "my_tag", SanitizePathToken("my tag"))
}

func TestSanitizeFileToken(t *testing.T) {
	assert.Equal(t, "a_b", SanitizeFileToken("a/b"))
	assert.Equal(t, "unnamed", SanitizeFileToken(""))
}

func TestCSVEscape(t *testing.T) {
	assert.Equal(t, "plain", CSVEscape("plain"))
	assert.Equal(t, `"a,b"`, CSVEscape("a,b"))
	assert.Equal(t, `"say ""hi"""`, CSVEscape(`say "hi"`))
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "abc", Truncate("abc", 10))
	assert.Equal(t, "ab…", Truncate("abcdef", 2))
	assert.Equal(t, "", Truncate("abc", 0))
	assert.Equal(t, "中…", Truncate("中文测试", 4))
}

func TestTruncateRunes(t *testing.T) {
	assert.Equal(t, "中文…", TruncateRunes("中文测试", 2))
	assert.Equal(t, "abc", TruncateRunes("abc", 10))
}

func TestItoaBoolFact(t *testing.T) {
	assert.Equal(t, "42", Itoa(42))
	assert.Equal(t, "42", Itoa64(42))
	assert.Equal(t, "true", BoolFact(true))
	assert.Equal(t, "false", BoolFact(false))
}
