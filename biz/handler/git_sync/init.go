package git_sync

import (
	"sync"

	synccore "github.com/yi-nology/git-sync-core"
)

var (
	once     sync.Once
	syncSvc  *synccore.Service
	initFunc func() *synccore.Service
)

func SetSyncServiceGetter(fn func() *synccore.Service) {
	initFunc = fn
}

func GetSyncService() *synccore.Service {
	once.Do(func() {
		if initFunc != nil {
			syncSvc = initFunc()
		}
	})
	return syncSvc
}
