// 运维/灾备/元数据 IDL。Req 字段与壳层 handler 请求体对齐,
// 生成 biz/model/ops 供 handler 直接绑定,避免手写 DTO 与 IDL 双份维护。

// ===== 元数据资产快照 =====

struct MetadataBackupReq {
    1: string repoKey (api.json="repo_key")
    2: optional bool withIssues (api.json="with_issues")
    3: optional bool withPRs (api.json="with_prs")
    4: optional bool withReleases (api.json="with_releases")
    5: optional bool withArchives (api.json="with_archives")
    6: optional bool withAssets (api.json="with_assets")
    7: optional bool withGists (api.json="with_gists")
    8: i32 maxItems (api.json="max_items")
    9: optional string since (api.json="since") // RFC3339 增量:仅保留 updatedAt>=since 的 issue/PR
}

struct MetadataSnapshot {
    1: string repoKey (api.json="repo_key")
    2: string platform (api.json="platform")
    3: string owner (api.json="owner")
    4: string repo (api.json="repo")
    5: string createdAt (api.json="created_at")
    6: string dir (api.json="dir")
    7: map<string, i32> counts (api.json="counts")
    8: list<string> files (api.json="files")
    9: list<string> archives (api.json="archives")
    10: list<string> assets (api.json="assets")
    11: list<string> warnings (api.json="warnings")
}

struct MetadataBackupResp {
    1: MetadataSnapshot snapshot (api.json="snapshot")
}

struct ListMetadataBackupsReq {
    1: optional string repoKey (api.query="repo_key" api.json="repo_key")
}

struct ListMetadataBackupsResp {
    1: list<MetadataSnapshot> items (api.json="items")
    2: i64 total (api.json="total")
}

struct BackupGistsReq {
    1: string platformKey (api.json="platform_key")
    2: i32 maxGists (api.json="max_gists")
}

struct BackupGistsResp {
    1: string platformKey (api.json="platform_key")
    2: i32 count (api.json="count")
    3: string dir (api.json="dir")
    4: list<string> warnings (api.json="warnings")
}

// ===== 元数据回灌 Restore =====
// kinds: labels,milestones,issues,prs,releases（空=全部可用分片）
// dry_run 缺省 true；overwrite=false 时同名跳过。

struct MetadataRestoreReq {
    1: string repoKey (api.json="repo_key")
    2: optional string snapshotDir (api.json="snapshot_dir")
    3: optional string targetPlatform (api.json="target_platform")
    4: optional string targetOwner (api.json="target_owner")
    5: optional string targetRepo (api.json="target_repo")
    6: list<string> kinds (api.json="kinds")
    7: optional bool dryRun (api.json="dry_run")
    8: optional bool overwrite (api.json="overwrite")
}

struct RestoreKindStat {
    1: i32 planned (api.json="planned")
    2: i32 created (api.json="created")
    3: i32 skipped (api.json="skipped")
    4: i32 failed (api.json="failed")
}

struct MetadataRestoreResult {
    1: string snapshotDir (api.json="snapshot_dir")
    2: string target (api.json="target")
    3: map<string, RestoreKindStat> stats (api.json="stats")
    4: list<string> warnings (api.json="warnings")
    5: bool dryRun (api.json="dry_run")
    6: string startedAt (api.json="started_at")
    7: string finishedAt (api.json="finished_at")
}

// ===== 灾备演练 =====

struct RunDRDrillReq {
    1: optional string name (api.json="name")
    2: optional bool all (api.json="all")
    3: i32 max (api.json="max")
}

struct DrillReport {
    1: string bundleName (api.json="bundle_name")
    2: string startedAt (api.json="started_at")
    3: string finishedAt (api.json="finished_at")
    4: i64 durationMs (api.json="duration_ms")
    5: bool success (api.json="success")
    6: list<string> refSpecs (api.json="ref_specs")
    7: list<string> missingRefs (api.json="missing_refs")
    8: bool fsckOk (api.json="fsck_ok")
    9: i32 commitCount (api.json="commit_count")
    10: string estRTO (api.json="est_rto")
    11: i64 bundleSize (api.json="bundle_size")
    12: list<string> errors (api.json="errors")
}

struct RunDRDrillResp {
    1: string mode (api.json="mode")
    2: list<DrillReport> reports (api.json="reports")
    3: map<string, string> summary (api.json="summary")
}

struct ExportDrillHistoryReq {
    1: optional string format (api.query="format" api.json="format")
    2: i32 limit (api.query="limit" api.json="limit")
}

// ===== RPO / 完整性 =====

struct RPOReportReq {
    1: optional i64 maxSeconds (api.query="max_seconds" api.json="max_seconds")
}

struct RPOMetric {
    1: string taskKey (api.json="task_key")
    2: string latestBundle (api.json="latest_bundle")
    3: string lastBackupAt (api.json="last_backup_at")
    4: i32 bundleCount (api.json="bundle_count")
    5: i64 totalSize (api.json="total_size")
    6: i64 rpoSeconds (api.json="rpo_seconds")
    7: string rpoHuman (api.json="rpo_human")
    8: bool rpoViolated (api.json="rpo_violated")
    9: string estRTO (api.json="est_rto")
}

struct RPOReport {
    1: string generated (api.json="generated")
    2: string backupDir (api.json="backup_dir")
    3: i64 rpoMaxSeconds (api.json="rpo_max_seconds")
    4: i64 overallRPOSeconds (api.json="overall_rpo_seconds")
    5: string overallRPOHuman (api.json="overall_rpo_human")
    6: string worstTask (api.json="worst_task")
    7: i32 violations (api.json="violations")
    8: list<RPOMetric> metrics (api.json="metrics")
}

// ===== 生命周期 =====

struct AutoDiscoverReq {
    1: string platformKey (api.json="platform_key")
    2: optional bool importNew (api.json="import_new")
    3: optional bool excludeArchived (api.json="exclude_archived")
    4: optional bool excludeForks (api.json="exclude_forks")
    5: i32 minStars (api.json="min_stars")
    6: string includeLanguage (api.json="include_language")
}

struct AutoDiscoverResp {
    1: string platformKey (api.json="platform_key")
    2: string scannedAt (api.json="scanned_at")
    3: i32 found (api.json="found")
    4: i32 existing (api.json="existing")
    5: list<string> newRepos (api.json="new_repos")
    6: i32 imported (api.json="imported")
    7: list<string> warnings (api.json="warnings")
}

struct DetectDriftReq {
    1: list<string> taskKeys (api.json="task_keys")
}

struct CleanupBackupsReq {
    1: string confirm (api.json="confirm")
}

// ===== Org 映射 / Starred / 公共 Org =====

struct OrgMirrorReq {
    1: string sourcePlatform (api.json="source_platform")
    2: string sourceOrg (api.json="source_org")
    3: string targetPlatform (api.json="target_platform")
    4: string targetOrg (api.json="target_org")
    5: string targetUser (api.json="target_user")
    6: string strategy (api.json="strategy") // preserve|single|flat|mixed
    7: optional bool dryRun (api.json="dry_run")
    8: optional bool createTasks (api.json="create_tasks")
    9: optional bool importNew (api.json="import_new")
}

struct OrgMirrorItem {
    1: string source (api.json="source")
    2: string target (api.json="target")
    3: string action (api.json="action") // planned|imported|task_created|skipped|failed
    4: string message (api.json="message")
}

struct OrgMirrorResp {
    1: string strategy (api.json="strategy")
    2: string sourceOrg (api.json="source_org")
    3: string target (api.json="target")
    4: bool dryRun (api.json="dry_run")
    5: i32 planned (api.json="planned")
    6: i32 imported (api.json="imported")
    7: i32 tasksCreated (api.json="tasks_created")
    8: list<OrgMirrorItem> items (api.json="items")
    9: list<string> warnings (api.json="warnings")
}

struct ImportStarredReq {
    1: string platformKey (api.json="platform_key")
    2: optional bool dryRun (api.json="dry_run")
    3: optional bool importNew (api.json="import_new")
    4: i32 max (api.json="max")
}

struct ImportPublicOrgReq {
    1: string platformKey (api.json="platform_key")
    2: string org (api.json="org")
    3: optional bool dryRun (api.json="dry_run")
    4: optional bool importNew (api.json="import_new")
    5: i32 max (api.json="max")
}

struct RepoImportPreview {
    1: string fullName (api.json="full_name")
    2: string cloneUrl (api.json="clone_url")
    3: bool fork (api.json="fork")
    4: bool archived (api.json="archived")
    5: i32 stars (api.json="stars")
}

struct ImportListResp {
    1: string source (api.json="source")
    2: bool dryRun (api.json="dry_run")
    3: i32 found (api.json="found")
    4: i32 imported (api.json="imported")
    5: list<RepoImportPreview> items (api.json="items")
    6: list<string> warnings (api.json="warnings")
}

// ===== 冷备 =====

struct ListBundlesReq {
    1: optional string taskKey (api.query="task_key" api.json="task_key")
}

struct VerifyBundleReq {
    1: string name (api.query="name" api.json="name")
}

struct RestoreBundleReq {
    1: string name (api.json="name")
    2: string destDir (api.json="dest_dir")
}

// ===== 重试 / 审计 =====

struct RetryRunReq {
    1: i32 runId (api.json="run_id")
}

struct BatchRetryReq {
    1: i32 limit (api.json="limit")
    2: optional string taskKey (api.json="task_key")
}

struct AuditReportReq {
    1: optional string startDate (api.json="start_date")
    2: optional string endDate (api.json="end_date")
    3: optional string action (api.json="action")
    4: optional string format (api.json="format")
    5: i32 limit (api.json="limit")
}

struct HealthScoreReq {
    1: i32 limit (api.json="limit")
}

// ===== 诊断 / 重建 / 部署密钥 =====

struct DiagnoseReq {
    1: i32 runId (api.json="run_id")
}

struct RebuildReq {
    1: string taskKey (api.json="task_key")
}

struct GenerateDeployKeyReq {
    1: optional string comment (api.json="comment")
}

// ===== Issues 导出 / 过滤导入 / Migration =====

struct ExportIssuesReq {
    1: string repoKey (api.json="repo_key")
    2: optional string state (api.json="state")
    3: i32 max (api.json="max")
    4: optional string format (api.json="format")
    5: optional bool withComments (api.json="with_comments")
}

struct SyncPlatformFilteredReq {
    1: string key (api.json="key")
    2: optional bool excludeArchived (api.json="exclude_archived")
    3: optional bool excludeForks (api.json="exclude_forks")
    4: i32 minStars (api.json="min_stars")
    5: optional string includeLanguage (api.json="include_language")
    6: optional string includeGlobs (api.json="include_globs")
    7: optional string excludeGlobs (api.json="exclude_globs")
}

struct ExportMigrationReq {
    1: string platformKey (api.json="platform_key")
    2: optional string org (api.json="org")
    3: optional bool wait (api.json="wait")
}

// ===== 策略模板 =====

struct PreviewTemplateReq {
    1: string templateId (api.json="template_id")
}

struct ApplyTemplateReq {
    1: string templateId (api.json="template_id")
    2: optional bool dryRun (api.json="dry_run")
}
