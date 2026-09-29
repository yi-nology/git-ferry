// 镜像中心 DTO。字段 JSON 保持 camelCase(与前端既有 API 一致),
// 故单独生成,不加 --snake_tag。

struct MirrorTargetReq {
    1: string remote (api.json="remote")
    2: string repoUrl (api.json="repoUrl")
    3: string targetModule (api.json="targetModule")
    4: string credType (api.json="credType")
    5: string credential (api.json="credential")
    6: string username (api.json="username")
}

struct CreateMirrorChannelReq {
    1: string name (api.json="name")
    2: string mode (api.json="mode")
    3: string repoKey (api.json="repoKey")
    4: list<MirrorTargetReq> targets (api.json="targets")
}

struct UpdateMirrorTargetReq {
    1: string remote (api.json="remote")
    2: string repoUrl (api.json="repoUrl")
    3: string targetModule (api.json="targetModule")
    4: string credType (api.json="credType")
    5: string credential (api.json="credential")
    6: string username (api.json="username")
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
