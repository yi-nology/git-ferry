package converter

import (
	olmodel "github.com/yi-nology/git-ferry/biz/model/operation_log"
	"github.com/yi-nology/git-ferry-core/model"
)

// ToOperationLogInfo 将审计日志模型转为 IDL 生成的对外结构 olmodel.OperationLogInfo。
func ToOperationLogInfo(l *model.OperationLog) *olmodel.OperationLogInfo {
	if l == nil {
		return nil
	}
	return &olmodel.OperationLogInfo{
		ID:       SafeUintToInt64(l.ID),
		Time:     l.CreatedAt.Format("2006-01-02 15:04:05"),
		User:     l.Actor,
		Action:   l.Action,
		Resource: l.Resource,
		Details:  l.Detail,
		IP:       l.IP,
	}
}

// ToOperationLogList 批量转换审计日志。
func ToOperationLogList(logs []*model.OperationLog) []*olmodel.OperationLogInfo {
	result := make([]*olmodel.OperationLogInfo, 0, len(logs))
	for _, l := range logs {
		result = append(result, ToOperationLogInfo(l))
	}
	return result
}
