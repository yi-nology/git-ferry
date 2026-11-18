package acp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEgressConfigParsing(t *testing.T) {
	assert.Equal(t, []string{"a.com", "*.b.com"}, splitList(" a.com, ,*.b.com "))
	assert.Empty(t, splitList("  , "))
	assert.Equal(t, []int{443, 80}, parsePorts(""))
	assert.Equal(t, []int{443, 8443}, parsePorts("443, nope, 8443"))
	assert.Equal(t, []int{443, 80}, parsePorts("0,99999"))
}
