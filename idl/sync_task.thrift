

struct SyncTaskInfo {
    1: i64 id (api.json="id")
    2: string key (api.json="key")
    3: string name (api.json="name")
    4: string sourceRepoKey (api.json="source_repo_key")
    5: string sourceBranch (api.json="source_branch")
    6: string targetRepoKey (api.json="target_repo_key")
    7: string targetBranch (api.json="target_branch")
    8: string syncMode (api.json="sync_mode")
    9: string cron (api.json="cron")
    10: string webhookToken (api.json="webhook_token")
    11: bool enabled (api.json="enabled")
    12: bool gitTags (api.json="git_tags")
    13: bool gitForce (api.json="git_force")
    14: bool gitPrune (api.json="git_prune")
    20: bool gitLfs (api.json="git_lfs")
    21: bool gitPushPrune (api.json="git_push_prune")
    22: bool keepDivergent (api.json="keep_divergent")
    23: string forcePushPolicy (api.json="force_push_policy")
    24: string includeBranches (api.json="include_branches")
    25: string excludeRefPatterns (api.json="exclude_ref_patterns")
    26: bool syncWiki (api.json="sync_wiki")
    27: bool gitBundle (api.json="git_bundle")
    28: bool submodules (api.json="submodules")
    16: string lastRunAt (api.json="last_run_at")
    17: string lastStatus (api.json="last_status")
    18: string createdAt (api.json="created_at")
}

struct ListTasksReq {
    1: string repoKey (api.query="repo_key")
    2: i32 page (api.query="page")
    3: i32 pageSize (api.query="page_size")
}

struct ListTasksResp {
    1: list<SyncTaskInfo> tasks (api.json="tasks")
    2: i64 total (api.json="total")
}

struct GetTaskReq {
    1: string key (api.query="key")
}

struct GetTaskResp {
    1: SyncTaskInfo task (api.json="task")
}

struct CreateTaskReq {
    1: string name (api.json="name")
    2: string sourceRepoKey (api.json="source_repo_key")
    3: string sourceBranch (api.json="source_branch")
    4: string targetRepoKey (api.json="target_repo_key")
    5: string targetBranch (api.json="target_branch")
    6: string syncMode (api.json="sync_mode")
    7: string cron (api.json="cron")
    8: bool gitTags (api.json="git_tags")
    9: bool gitForce (api.json="git_force")
    10: bool gitPrune (api.json="git_prune")
    15: bool gitLfs (api.json="git_lfs")
    16: bool gitPushPrune (api.json="git_push_prune")
    17: optional bool keepDivergent (api.json="keep_divergent")
    18: optional string forcePushPolicy (api.json="force_push_policy")
    19: optional string includeBranches (api.json="include_branches")
    20: optional string excludeRefPatterns (api.json="exclude_ref_patterns")
    21: bool syncWiki (api.json="sync_wiki")
    22: bool gitBundle (api.json="git_bundle")
    23: bool submodules (api.json="submodules")
}

struct CreateTaskResp {
    1: SyncTaskInfo task (api.json="task")
}

struct UpdateTaskReq {
    1: string key (api.json="key")
    2: string name (api.json="name")
    3: string sourceBranch (api.json="source_branch")
    4: string targetBranch (api.json="target_branch")
    5: string syncMode (api.json="sync_mode")
    6: string cron (api.json="cron")
    7: bool enabled (api.json="enabled")
    8: bool gitTags (api.json="git_tags")
    9: bool gitForce (api.json="git_force")
    10: bool gitPrune (api.json="git_prune")
    15: bool gitLfs (api.json="git_lfs")
    16: bool gitPushPrune (api.json="git_push_prune")
    17: optional bool keepDivergent (api.json="keep_divergent")
    18: optional string forcePushPolicy (api.json="force_push_policy")
    19: optional string includeBranches (api.json="include_branches")
    20: optional string excludeRefPatterns (api.json="exclude_ref_patterns")
    21: bool syncWiki (api.json="sync_wiki")
    22: bool gitBundle (api.json="git_bundle")
    23: bool submodules (api.json="submodules")
}

struct UpdateTaskResp {
    1: SyncTaskInfo task (api.json="task")
}

struct DeleteTaskReq {
    1: string key (api.query="key")
}

struct DeleteTaskResp {
    1: bool success (api.json="success")
}

struct RunTaskReq {
    1: string key (api.query="key")
}

struct RunTaskResp {
    1: bool success (api.json="success")
    2: string message (api.json="message")
}

struct PreviewSyncReq {
    1: string sourceRepoKey (api.json="source_repo_key")
    2: string sourceBranch (api.json="source_branch")
    3: string targetRepoKey (api.json="target_repo_key")
    4: string targetBranch (api.json="target_branch")
}

struct PreviewSyncResp {
    1: bool canSync (api.json="can_sync")
    2: bool sourceExists (api.json="source_exists")
    3: bool targetExists (api.json="target_exists")
    6: string message (api.json="message")
}

struct SyncRunInfo {
    1: i64 id (api.json="id")
    2: string taskKey (api.json="task_key")
    3: string triggerSource (api.json="trigger_source")
    4: string status (api.json="status")
    5: string startTime (api.json="start_time")
    6: string endTime (api.json="end_time")
    7: string commitRange (api.json="commit_range")
    8: string details (api.json="details")
    9: string errorMessage (api.json="error_message")
    10: string createdAt (api.json="created_at")
    11: optional i64 webhookEventId (api.json="webhook_event_id")
    12: i64 durationMs (api.json="duration_ms")
    13: string errorType (api.json="error_type")
    14: i32 retryTotal (api.json="retry_total")
    15: list<SyncRunStepInfo> steps (api.json="steps")
}

struct SyncRunStepInfo {
    1: i64 id (api.json="id")
    2: string stepName (api.json="step_name")
    3: string status (api.json="status")
    4: string startTime (api.json="start_time")
    5: string endTime (api.json="end_time")
    6: i64 durationMs (api.json="duration_ms")
    7: string errorMsg (api.json="error_msg")
    8: string errorType (api.json="error_type")
    9: i32 retryCount (api.json="retry_count")
}

struct ListHistoryReq {
    1: string taskKey (api.query="task_key")
    2: i32 limit (api.query="limit")
}

struct ListHistoryResp {
    1: list<SyncRunInfo> runs (api.json="runs")
}

struct DeleteHistoryReq {
    1: i64 id (api.query="id")
}

struct DeleteHistoryResp {
    1: string message (api.json="message")
}
