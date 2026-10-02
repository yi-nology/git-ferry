package git_sync

import (
	"context"
	"github.com/yi-nology/git-ferry/biz/model/ops"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// GenerateDeployKey POST /api/v1/ops/deploy-key
// 生成 Ed25519 密钥对:私钥交给镜像任务,公钥粘贴到目标平台 deploy key。
// 私钥只在响应中返回一次,不落库(调用方自行存入密钥管理)。
func GenerateDeployKey(ctx context.Context, c *app.RequestContext) {
	var req ops.GenerateDeployKeyReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	comment := optStr(req.Comment)
	if comment == "" {
		comment = "gitferry-mirror"
	}
	key, err := corebridge.GenerateDeployKey(comment)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "generate_deploy_key", "deploy_key", comment, "生成镜像部署密钥 "+comment)
	response.Success(c, map[string]any{
		"private_key_pem": key.PrivateKeyPEM,
		"public_key":      key.PublicKeyAuthorized,
		"fingerprint":     key.Fingerprint,
		"comment":         comment,
		// 提醒:私钥仅此一次返回
		"note": "请立即保存私钥;服务端不持久化,丢失需重新生成",
	})
}
