package converter

import (
	"github.com/yi-nology/git-ferry-core/model"
	olmodel "github.com/yi-nology/git-ferry/biz/model/operation_log"
)

// operationLogInfoSpec:core OperationLog → DTO OperationLogInfo 的搬运契约。
//
// 审计日志这三处是"概念同名、字段名不同构",归一化后对不上,只能显式声明别名。
// 别名方向:DTO 字段名 → core 字段名。
var operationLogInfoSpec = fieldSpec{
	layout: layoutDateTime,
	aliases: map[string]string{
		"time":    "createdat", // DTO 的 Time 就是 core 的 CreatedAt
		"user":    "actor",     // DTO 的 User 就是 core 的 Actor
		"details": "detail",    // DTO 的 Details 就是 core 的 Detail
	},
}

// ToOperationLogInfo 将审计日志模型转为 IDL 生成的对外结构 olmodel.OperationLogInfo。
func ToOperationLogInfo(l *model.OperationLog) *olmodel.OperationLogInfo {
	if l == nil {
		return nil
	}
	info := &olmodel.OperationLogInfo{}
	mapFields(info, l, operationLogInfoSpec)
	return info
}

// ToOperationLogList 批量转换审计日志。
func ToOperationLogList(logs []*model.OperationLog) []*olmodel.OperationLogInfo {
	result := make([]*olmodel.OperationLogInfo, 0, len(logs))
	for _, l := range logs {
		result = append(result, ToOperationLogInfo(l))
	}
	return result
}
