package git_sync

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/stretchr/testify/require"
)

func TestAuthUserRoundTrip(t *testing.T) {
	c := app.NewContext(0)
	require.Equal(t, "", GetAuthUser(c))

	SetAuthUser(c, "alice")
	require.Equal(t, "alice", GetAuthUser(c))

	SetAuthUser(c, "")
	require.Equal(t, "alice", GetAuthUser(c), "empty set is no-op")
}
