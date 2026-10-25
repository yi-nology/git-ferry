

struct WebhookRuleInfo {
    1: i64 id (api.json="id")
    2: string name (api.json="name")
    3: string repoKey (api.json="repo_key")
    4: string eventType (api.json="event_type")
    5: string branchPattern (api.json="branch_pattern")
    6: string action (api.json="action")
    7: string syncTaskKeys (api.json="sync_task_keys")
    8: i32 minInterval (api.json="min_interval")
    9: bool enabled (api.json="enabled")
    10: string description (api.json="description")
    11: string createdAt (api.json="created_at")
}

struct ListRulesReq {
    1: string repoKey (api.query="repo_key")
}

struct ListRulesResp {
    1: list<WebhookRuleInfo> rules (api.json="rules")
}

struct GetRuleReq {
    1: i64 id (api.query="id")
}

struct GetRuleResp {
    1: WebhookRuleInfo rule (api.json="rule")
}

struct CreateRuleReq {
    1: string name (api.json="name")
    2: string repoKey (api.json="repo_key")
    3: string eventType (api.json="event_type")
    4: string branchPattern (api.json="branch_pattern")
    5: string action (api.json="action")
    6: string syncTaskKeys (api.json="sync_task_keys")
    // optional:不传=用默认值(60);显式 0=不限流。
    7: optional i32 minInterval (api.json="min_interval")
    // optional:不传=默认启用;显式 false=建出来就是禁用的。
    8: optional bool enabled (api.json="enabled")
    9: string description (api.json="description")
}

struct CreateRuleResp {
    1: WebhookRuleInfo rule (api.json="rule")
}

struct UpdateRuleReq {
    1: i64 id (api.json="id")
    2: string name (api.json="name")
    3: string eventType (api.json="event_type")
    // optional:不传=不修改;显式 ""=匹配全部分支。
    4: optional string branchPattern (api.json="branch_pattern")
    5: string action (api.json="action")
    6: string syncTaskKeys (api.json="sync_task_keys")
    // optional:不传=不修改;显式 0=不限流。
    7: optional i32 minInterval (api.json="min_interval")
    // optional:不传=不修改;显式 false=禁用。
    8: optional bool enabled (api.json="enabled")
    // optional:不传=不修改;显式 ""=清空描述。
    9: optional string description (api.json="description")
}

struct UpdateRuleResp {
    1: WebhookRuleInfo rule (api.json="rule")
}

struct DeleteRuleReq {
    1: i64 id (api.query="id")
}

struct DeleteRuleResp {
    1: bool success (api.json="success")
}

struct WebhookEventInfo {
    1: i64 id (api.json="id")
    2: string eventId (api.json="event_id")
    3: string repoKey (api.json="repo_key")
    4: string eventType (api.json="event_type")
    5: string source (api.json="source")
    6: string actorName (api.json="actor_name")
    7: string branch (api.json="branch")
    8: string commitSha (api.json="commit_sha")
    9: string status (api.json="status")
    10: string errorMessage (api.json="error_message")
    11: string processedAt (api.json="processed_at")
    12: string createdAt (api.json="created_at")
}

struct ListEventsReq {
    1: string repoKey (api.query="repo_key")
    2: i32 limit (api.query="limit")
}

struct ListEventsResp {
    1: list<WebhookEventInfo> events (api.json="events")
}

struct RetryEventReq {
    1: i64 id (api.query="id")
}

struct RetryEventResp {
    1: bool success (api.json="success")
    2: string message (api.json="message")
}

struct PlatformWebhookInfo {
    1: i64 id (api.json="id")
    2: string url (api.json="url")
    3: list<string> events (api.json="events")
}

struct RegisterPlatformWebhookReq {
    1: string repoKey (api.json="repo_key")
    2: string callbackUrl (api.json="callback_url")
    3: string secret (api.json="secret")
    4: list<string> events (api.json="events")
}

struct RegisterPlatformWebhookResp {
    1: PlatformWebhookInfo webhook (api.json="webhook")
}

struct ListPlatformWebhooksReq {
    1: string repoKey (api.query="repo_key")
}

struct ListPlatformWebhooksResp {
    1: list<PlatformWebhookInfo> webhooks (api.json="webhooks")
}

struct DeletePlatformWebhookReq {
    1: string repoKey (api.query="repo_key")
    2: i64 webhookId (api.query="webhook_id")
}

struct DeletePlatformWebhookResp {
    1: bool success (api.json="success")
}
