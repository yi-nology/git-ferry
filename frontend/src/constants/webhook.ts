/**
 * Webhook 接收端点(后端唯一注册路径,见 biz/router/custom.go)。
 * repo_key 为仓库 key;配到 Git 平台的回调 URL 必须用这个前缀,
 * 历史上曾误用 /api/v1/webhook/receive 导致 404(已修,勿改回)。
 */
export const WEBHOOK_RECEIVE_PATH = '/api/webhook/receive'

export function buildWebhookReceiveUrl(repoKey: string): string {
  return `${window.location.origin}${WEBHOOK_RECEIVE_PATH}/${repoKey}`
}
