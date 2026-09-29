include "base.thrift"
include "repo.thrift"
include "sync_task.thrift"
include "webhook.thrift"
include "operation_log.thrift"
include "platform.thrift"
include "system.thrift"
include "ops.thrift"

service RepoService {
    repo.ListReposResp RepoList(1: repo.ListReposReq req) (api.get="/api/v1/repos")
    repo.GetRepoResp RepoGet(1: repo.GetRepoReq req) (api.get="/api/v1/repo")
    repo.CreateRepoResp RepoCreate(1: repo.CreateRepoReq req) (api.post="/api/v1/repo/create")
    repo.UpdateRepoResp RepoUpdate(1: repo.UpdateRepoReq req) (api.post="/api/v1/repo/update")
    repo.DeleteRepoResp RepoDelete(1: repo.DeleteRepoReq req) (api.post="/api/v1/repo/delete")
    repo.TestConnectionResp RepoTest(1: repo.TestConnectionReq req) (api.post="/api/v1/repo/test")
    repo.ListBranchesResp RepoBranches(1: repo.ListBranchesReq req) (api.get="/api/v1/repo/branches")
    repo.BatchReposResp BatchRepos(1: repo.BatchReposReq req) (api.post="/api/v1/repos/batch")
}

service SyncTaskService {
    sync_task.ListTasksResp TaskList(1: sync_task.ListTasksReq req) (api.get="/api/v1/sync/tasks")
    sync_task.GetTaskResp TaskGet(1: sync_task.GetTaskReq req) (api.get="/api/v1/sync/task")
    sync_task.CreateTaskResp TaskCreate(1: sync_task.CreateTaskReq req) (api.post="/api/v1/sync/task/create")
    sync_task.UpdateTaskResp TaskUpdate(1: sync_task.UpdateTaskReq req) (api.post="/api/v1/sync/task/update")
    sync_task.DeleteTaskResp TaskDelete(1: sync_task.DeleteTaskReq req) (api.post="/api/v1/sync/task/delete")
    sync_task.RunTaskResp TaskRun(1: sync_task.RunTaskReq req) (api.post="/api/v1/sync/task/run")
    sync_task.PreviewSyncResp TaskPreview(1: sync_task.PreviewSyncReq req) (api.post="/api/v1/sync/preview")
    sync_task.ListHistoryResp TaskHistory(1: sync_task.ListHistoryReq req) (api.get="/api/v1/sync/history")
    sync_task.DeleteHistoryResp DeleteHistory(1: sync_task.DeleteHistoryReq req) (api.post="/api/v1/sync/history/delete")
}

service WebhookService {
    webhook.ListRulesResp RuleList(1: webhook.ListRulesReq req) (api.get="/api/v1/webhook/rules")
    webhook.GetRuleResp RuleGet(1: webhook.GetRuleReq req) (api.get="/api/v1/webhook/rule")
    webhook.CreateRuleResp RuleCreate(1: webhook.CreateRuleReq req) (api.post="/api/v1/webhook/rule/create")
    webhook.UpdateRuleResp RuleUpdate(1: webhook.UpdateRuleReq req) (api.post="/api/v1/webhook/rule/update")
    webhook.DeleteRuleResp RuleDelete(1: webhook.DeleteRuleReq req) (api.post="/api/v1/webhook/rule/delete")
    webhook.ListEventsResp ListEvents(1: webhook.ListEventsReq req) (api.get="/api/v1/webhook/events")
    webhook.RetryEventResp RetryEvent(1: webhook.RetryEventReq req) (api.post="/api/v1/webhook/event/retry")
    webhook.RegisterPlatformWebhookResp RegisterPlatformWebhook(1: webhook.RegisterPlatformWebhookReq req) (api.post="/api/v1/webhook/platform/register")
    webhook.ListPlatformWebhooksResp ListPlatformWebhooks(1: webhook.ListPlatformWebhooksReq req) (api.get="/api/v1/webhook/platform/list")
    webhook.DeletePlatformWebhookResp DeletePlatformWebhook(1: webhook.DeletePlatformWebhookReq req) (api.post="/api/v1/webhook/platform/delete")
}

service OperationLogService {
    operation_log.ListOperationLogsResp ListOperationLogs(1: operation_log.ListOperationLogsReq req) (api.get="/api/v1/logs/operations")
}

service PlatformService {
    platform.ListPlatformsResp ListPlatforms(1: platform.ListPlatformsReq req) (api.get="/api/v1/platforms")
    platform.GetPlatformResp GetPlatform(1: platform.GetPlatformReq req) (api.get="/api/v1/platform")
    platform.CreatePlatformResp CreatePlatform(1: platform.CreatePlatformReq req) (api.post="/api/v1/platform/create")
    platform.UpdatePlatformResp UpdatePlatform(1: platform.UpdatePlatformReq req) (api.post="/api/v1/platform/update")
    platform.DeletePlatformResp DeletePlatform(1: platform.DeletePlatformReq req) (api.post="/api/v1/platform/delete")
    platform.SetDefaultPlatformResp SetDefaultPlatform(1: platform.SetDefaultPlatformReq req) (api.post="/api/v1/platform/set-default")
    platform.TestPlatformConnectionResp TestPlatformConnection(1: platform.TestPlatformConnectionReq req) (api.post="/api/v1/platform/test")
    platform.ListPlatformReposResp ListPlatformRepos(1: platform.ListPlatformReposReq req) (api.get="/api/v1/platform/repos")
    platform.SyncPlatformReposResp SyncPlatformRepos(1: platform.SyncPlatformReposReq req) (api.post="/api/v1/platform/sync-repos")
}

service SystemService {
    system.SystemStatusData SystemStatus(1: system.SystemStatusReq req) (api.get="/api/v1/system/status")
}
