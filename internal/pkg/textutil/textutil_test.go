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

func TestCSVEscape(t *testing.T) {
	assert.Equal(t, "plain", CSVEscape("plain"))
	assert.Equal(t, `"a,b"`, CSVEscape("a,b"))
	assert.Equal(t, `"say ""hi"""`, CSVEscape(`say "hi"`))
}
