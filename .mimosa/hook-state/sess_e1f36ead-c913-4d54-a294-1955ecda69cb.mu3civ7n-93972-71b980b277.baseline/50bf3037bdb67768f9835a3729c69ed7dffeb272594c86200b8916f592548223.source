package git_sync

import (
	"sync"

	"github.com/yi-nology/git-sync-service/internal/corebridge"
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
