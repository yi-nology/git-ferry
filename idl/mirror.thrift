// 镜像中心 DTO。字段 JSON 保持 camelCase(与前端既有 API 一致),
// 故单独生成,不加 --snake_tag。

struct MirrorTargetReq {
    1: string remote (api.json="remote")
    2: string repoUrl (api.json="repoUrl")
    3: string targetModule (api.json="targetModule")
    4: string credType (api.json="credType")
    5: string credential (api.json="credential")
    6: string username (api.json="username")
    // SSH 主机密钥钉扎:目标侧之前没法钉,于是同一次发布里
    // 「检查路径」fail-closed、「推送路径」TOFU,两套策略相反。
    7: bool insecureSkipTLS (api.json="insecureSkipTLS")
    8: string hostKeyFingerprint (api.json="hostKeyFingerprint")
    9: string knownHostsPath (api.json="knownHostsPath")
}

struct CreateMirrorChannelReq {
    1: string name (api.json="name")
    2: string mode (api.json="mode")
    3: string repoKey (api.json="repoKey")
    4: list<MirrorTargetReq> targets (api.json="targets")
    // 痕迹清理规则:发布前把内容里的内网痕迹清掉。
    // 身份改写只管 module path;内网 registry / 部署代号 / 令牌
    // 原样进公开 module 就是泄漏。
    5: list<MirrorScrubRuleReq> scrubRules (api.json="scrubRules")
}

// 痕迹清理规则。severity=mandatory 表示命中即中止发布(密钥类)。
struct MirrorScrubRuleReq {
    1: string name (api.json="name")
    2: string category (api.json="category")
    3: string pattern (api.json="pattern")
    4: string severity (api.json="severity")
    5: string replacement (api.json="replacement")
}

struct UpdateMirrorTargetReq {
    1: string remote (api.json="remote")
    2: string repoUrl (api.json="repoUrl")
    3: string targetModule (api.json="targetModule")
    4: string credType (api.json="credType")
    5: string credential (api.json="credential")
    6: string username (api.json="username")
    7: bool insecureSkipTLS (api.json="insecureSkipTLS")
    8: string hostKeyFingerprint (api.json="hostKeyFingerprint")
    9: string knownHostsPath (api.json="knownHostsPath")
}

struct MirrorPreviewReq {
    1: i32 targetId (api.json="targetId")
    2: string tag (api.json="tag")
}

struct MirrorRunReq {
    1: i32 targetId (api.json="targetId")
    2: list<string> tags (api.json="tags")
    3: bool allowOverwrite (api.json="allowOverwrite")
}

struct MirrorVerifyReq {
    1: i32 targetId (api.json="targetId")
    2: string tag (api.json="tag")
}
