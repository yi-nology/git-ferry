package git_sync

import (
	"sync"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

var (
	once     sync.Once
	syncSvc  *corebridge.Service
	initFunc func() *corebridge.Service
)

func SetSyncServiceGetter(fn func() *corebridge.Service) {
	initFunc = fn
}

func GetSyncService() *corebridge.Service {
	once.Do(func() {
		if initFunc != nil {
			syncSvc = initFunc()
		}
	})
	return syncSvc
}

// requireSyncService 在 Service 未就绪时写 503 并返回 ok=false,
// 供 handler 在解引用前短路,避免启动时序导致 panic。
func requireSyncService(c *app.RequestContext) (*corebridge.Service, bool) {
	svc := GetSyncService()
	if svc == nil {
		response.Error(c, consts.StatusServiceUnavailable, "service unavailable")
		return nil, false
	}
	return svc, true
}
