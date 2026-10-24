package converter

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/yi-nology/git-ferry-core/model"
)

// 黄金 JSON 基线。
//
// 这批期望值是**重构前**的实现跑出来的,重构后逐字比对——
// 对外 DTO 的 JSON 形状(字段名、字段顺序、零值处理、omitempty 行为)
// 是本包唯一不可变的契约,任何搬运机制的改动都不许改动它。
//
// 覆盖两种样本:
//   - 满值样本:每个 core 字段都填上非零值,能看出"哪个字段没搬过去";
//   - 零值样本:core 实体全零,钉住空串 / null / 字段缺省等零值行为。

// goldenTime 固定时间,保证基线可复现。
func goldenTime() time.Time { return time.Date(2024, 5, 17, 8, 30, 15, 0, time.UTC) }

func goldenTask() *model.SyncTask {
	tt := goldenTime()
	return &model.SyncTask{
		ID: 11, Key: "task-key", Name: "Task Name",
		SourceRepoKey: "src-repo", SourceBranch: "main",
		TargetRepoKey: "dst-repo", TargetBranch: "release",
		SyncMode: "mirror", Cron: "0 * * * *", WebhookToken: "secret-token",
		Enabled: true, GitTags: true, GitForce: true, GitPrune: true,
		GitLFS: true, SyncWiki: true, GitBundle: true, Submodules: true,
		GitPushPrune: true, KeepDivergent: true,
		ForcePushPolicy: "allow", IncludeBranches: "main,release/*",
		ExcludeRefPatterns: "refs/pull/*",
		LastRunAt:          &tt, LastStatus: "success", CreatedAt: tt,
	}
}

func goldenRun() *model.SyncRun {
	tt := goldenTime()
	eventID := uint(91)
	return &model.SyncRun{
		ID: 21, TaskKey: "task-key", TriggerSource: "manual", Status: "success",
		StartTime: tt, EndTime: &tt, CommitRange: "abc..def", Details: "details text",
		ErrorMessage: "boom", WebhookEventID: &eventID, DurationMs: 1234,
		ErrorType: "network", RetryTotal: 2, CreatedAt: tt,
		Steps: []model.SyncRunStep{{
			ID: 31, RunID: 22, StepName: "fetch", Status: "success",
			StartTime: tt, EndTime: &tt, DurationMs: 55, ErrorMsg: "step err",
			ErrorType: "net", Output: "out", RetryCount: 1, CreatedAt: tt,
		}},
	}
}

func goldenStep() *model.SyncRunStep {
	tt := goldenTime()
	return &model.SyncRunStep{
		ID: 31, RunID: 22, StepName: "fetch", Status: "success",
		StartTime: tt, EndTime: &tt, DurationMs: 55, ErrorMsg: "step err",
		ErrorType: "net", Output: "out", RetryCount: 1, CreatedAt: tt,
	}
}

func goldenPlatform() *model.Platform {
	tt := goldenTime()
	return &model.Platform{
		ID: 51, Key: "github", Name: "GitHub", Type: "github",
		InstanceURL: "https://github.com", APIURL: "https://api.github.com",
		AccessToken: "token", GitHubAppID: 42, GitHubInstallationID: 7,
		GitHubPrivateKey: "PEM", SkipTLSVerify: true, CACertPath: "/ca.pem",
		ProxyURL: "http://proxy", SSHHostKeyFingerprint: "SHA256:xx",
		SSHKnownHostsPath: "/known_hosts", IsDefault: true, Status: "active",
		LastTestAt: &tt, LastTestResult: "success", RepoCount: 10,
		CreatedAt: tt, UpdatedAt: tt,
	}
}

func goldenRepo() *model.Repo {
	tt := goldenTime()
	return &model.Repo{
		ID: 41, Key: "repo-key", Name: "Repo Name", PlatformID: 3,
		Platform: "github", PlatformOwner: "owner", PlatformRepo: "repo",
		CloneURL: "https://github.com/owner/repo.git", SSHURL: "git@github.com:owner/repo.git",
		DefaultBranch: "main", AccessToken: "tok", WebhookSecret: "s", WebhookID: 9,
		Status: "active", CreatedAt: tt, UpdatedAt: tt,
	}
}

func goldenRule() *model.WebhookRule {
	tt := goldenTime()
	return &model.WebhookRule{
		ID: 61, Name: "Rule", RepoKey: "repo-key", EventType: "push",
		BranchPattern: "main", Action: "sync", MinInterval: 60, Enabled: true,
		Description: "desc", CreatedAt: tt, UpdatedAt: tt,
		Tasks: []model.WebhookRuleTask{{TaskKey: "t1"}, {TaskKey: "t2"}},
	}
}

func goldenEvent() *model.WebhookEvent {
	tt := goldenTime()
	return &model.WebhookEvent{
		ID: 71, EventID: "evt-1", RepoKey: "repo-key", EventType: "push",
		Source: "github", ActorName: "user", Branch: "main", CommitSHA: "abc123",
		Payload: []byte("{}"), Status: "processed", ErrorMessage: "err",
		ProcessedAt: &tt, CreatedAt: tt,
	}
}

func goldenOperationLog() *model.OperationLog {
	tt := goldenTime()
	return &model.OperationLog{
		ID: 81, Action: "create", ResourceType: "repo", ResourceKey: "repo-key",
		Resource: "创建仓库 repo-key", Actor: "user", IP: "1.2.3.4", Status: "success",
		Detail: "detail text", PrevHash: "prev", EntryHash: "entry", CreatedAt: tt,
	}
}

// TestGoldenJSONShape 钉死对外 DTO 的 JSON 形状。
//
// 期望值取自重构前的实现,重构只换搬运机制、不换输出。
func TestGoldenJSONShape(t *testing.T) {
	cases := []struct {
		name string
		got  any
		want string
	}{
		{
			name: "TaskInfo",
			got:  ToTaskInfo(goldenTask()),
			want: `{"id":11,"key":"task-key","name":"Task Name","source_repo_key":"src-repo","source_branch":"main","target_repo_key":"dst-repo","target_branch":"release","sync_mode":"mirror","cron":"0 * * * *","webhook_token":"","enabled":true,"git_tags":true,"git_force":true,"git_prune":true,"git_lfs":true,"git_push_prune":true,"keep_divergent":true,"force_push_policy":"allow","include_branches":"main,release/*","exclude_ref_patterns":"refs/pull/*","sync_wiki":true,"git_bundle":true,"submodules":true,"last_run_at":"2024-05-17 08:30:15","last_status":"success","created_at":"2024-05-17 08:30:15"}`,
		},
		{
			name: "TaskInfoZero",
			got:  ToTaskInfo(&model.SyncTask{}),
			want: `{"id":0,"key":"","name":"","source_repo_key":"","source_branch":"","target_repo_key":"","target_branch":"","sync_mode":"","cron":"","webhook_token":"","enabled":false,"git_tags":false,"git_force":false,"git_prune":false,"git_lfs":false,"git_push_prune":false,"keep_divergent":false,"force_push_policy":"","include_branches":"","exclude_ref_patterns":"","sync_wiki":false,"git_bundle":false,"submodules":false,"last_run_at":"","last_status":"","created_at":"0001-01-01 00:00:00"}`,
		},
		{
			name: "SyncRunInfo",
			got:  ToSyncRunInfo(goldenRun()),
			want: `{"id":21,"task_key":"task-key","trigger_source":"manual","status":"success","start_time":"2024-05-17 08:30:15","end_time":"2024-05-17 08:30:15","commit_range":"abc..def","details":"details text","error_message":"boom","created_at":"2024-05-17 08:30:15","webhook_event_id":91,"duration_ms":1234,"error_type":"network","retry_total":2,"steps":[{"id":31,"step_name":"fetch","status":"success","start_time":"2024-05-17 08:30:15","end_time":"2024-05-17 08:30:15","duration_ms":55,"error_msg":"step err","error_type":"net","retry_count":1}]}`,
		},
		{
			name: "SyncRunInfoZero",
			got:  ToSyncRunInfo(&model.SyncRun{}),
			want: `{"id":0,"task_key":"","trigger_source":"","status":"","start_time":"0001-01-01 00:00:00","end_time":"","commit_range":"","details":"","error_message":"","created_at":"0001-01-01 00:00:00","duration_ms":0,"error_type":"","retry_total":0,"steps":null}`,
		},
		{
			name: "SyncRunInfoLight",
			got:  ToSyncRunInfoListLight([]*model.SyncRun{goldenRun()})[0],
			want: `{"id":21,"task_key":"task-key","trigger_source":"manual","status":"success","start_time":"2024-05-17 08:30:15","end_time":"2024-05-17 08:30:15","commit_range":"abc..def","details":"","error_message":"boom","created_at":"2024-05-17 08:30:15","duration_ms":1234,"error_type":"network","retry_total":2,"steps":null}`,
		},
		{
			name: "SyncRunInfoLightZero",
			got:  ToSyncRunInfoListLight([]*model.SyncRun{{}})[0],
			want: `{"id":0,"task_key":"","trigger_source":"","status":"","start_time":"0001-01-01 00:00:00","end_time":"","commit_range":"","details":"","error_message":"","created_at":"0001-01-01 00:00:00","duration_ms":0,"error_type":"","retry_total":0,"steps":null}`,
		},
		{
			name: "SyncRunStepInfo",
			got:  ToSyncRunStepInfo(goldenStep()),
			want: `{"id":31,"step_name":"fetch","status":"success","start_time":"2024-05-17 08:30:15","end_time":"2024-05-17 08:30:15","duration_ms":55,"error_msg":"step err","error_type":"net","retry_count":1}`,
		},
		{
			name: "SyncRunStepInfoZero",
			got:  ToSyncRunStepInfo(&model.SyncRunStep{}),
			want: `{"id":0,"step_name":"","status":"","start_time":"0001-01-01 00:00:00","end_time":"","duration_ms":0,"error_msg":"","error_type":"","retry_count":0}`,
		},
		{
			name: "PlatformInfo",
			got:  ToPlatformInfo(goldenPlatform()),
			want: `{"id":51,"key":"github","name":"GitHub","type":"github","instance_url":"https://github.com","api_url":"https://api.github.com","skip_tls_verify":true,"ca_cert_path":"/ca.pem","ssh_host_key_fingerprint":"SHA256:xx","ssh_known_hosts_path":"/known_hosts","proxy_url":"http://proxy","is_default":true,"status":"active","last_test_result":"success","repo_count":10,"created_at":"2024-05-17T08:30:15Z","last_test_at":"2024-05-17T08:30:15Z","updated_at":"2024-05-17T08:30:15Z","has_token":true,"has_github_app":true,"github_app_id":42}`,
		},
		{
			name: "PlatformInfoZero",
			got:  ToPlatformInfo(&model.Platform{}),
			want: `{"id":0,"key":"","name":"","type":"","instance_url":"","api_url":"","skip_tls_verify":false,"ca_cert_path":"","ssh_host_key_fingerprint":"","ssh_known_hosts_path":"","proxy_url":"","is_default":false,"status":"","last_test_result":"","repo_count":0,"created_at":"0001-01-01T00:00:00Z","last_test_at":"","updated_at":"0001-01-01T00:00:00Z","has_token":false,"has_github_app":false,"github_app_id":0}`,
		},
		{
			name: "RepoInfo",
			got:  ToRepoInfo(goldenRepo()),
			want: `{"id":41,"key":"repo-key","name":"Repo Name","platform":"github","platform_owner":"owner","platform_repo":"repo","clone_url":"https://github.com/owner/repo.git","ssh_url":"git@github.com:owner/repo.git","default_branch":"main","status":"active","created_at":"2024-05-17T08:30:15Z"}`,
		},
		{
			name: "RepoInfoZero",
			got:  ToRepoInfo(&model.Repo{}),
			want: `{"id":0,"key":"","name":"","platform":"","platform_owner":"","platform_repo":"","clone_url":"","ssh_url":"","default_branch":"","status":"","created_at":"0001-01-01T00:00:00Z"}`,
		},
		{
			name: "RuleInfo",
			got:  ToRuleInfo(goldenRule()),
			want: `{"id":61,"name":"Rule","repo_key":"repo-key","event_type":"push","branch_pattern":"main","action":"sync","sync_task_keys":"t1,t2","min_interval":60,"enabled":true,"description":"desc","created_at":"2024-05-17 08:30:15"}`,
		},
		{
			name: "RuleInfoZero",
			got:  ToRuleInfo(&model.WebhookRule{}),
			want: `{"id":0,"name":"","repo_key":"","event_type":"","branch_pattern":"","action":"","sync_task_keys":"","min_interval":0,"enabled":false,"description":"","created_at":"0001-01-01 00:00:00"}`,
		},
		{
			name: "EventInfo",
			got:  ToEventInfo(goldenEvent()),
			want: `{"id":71,"event_id":"evt-1","repo_key":"repo-key","event_type":"push","source":"github","actor_name":"user","branch":"main","commit_sha":"abc123","status":"processed","error_message":"err","processed_at":"2024-05-17 08:30:15","created_at":"2024-05-17 08:30:15"}`,
		},
		{
			name: "EventInfoZero",
			got:  ToEventInfo(&model.WebhookEvent{}),
			want: `{"id":0,"event_id":"","repo_key":"","event_type":"","source":"","actor_name":"","branch":"","commit_sha":"","status":"","error_message":"","processed_at":"","created_at":"0001-01-01 00:00:00"}`,
		},
		{
			name: "OperationLogInfo",
			got:  ToOperationLogInfo(goldenOperationLog()),
			want: `{"id":81,"time":"2024-05-17 08:30:15","user":"user","action":"create","resource":"创建仓库 repo-key","details":"detail text","ip":"1.2.3.4"}`,
		},
		{
			name: "OperationLogInfoZero",
			got:  ToOperationLogInfo(&model.OperationLog{}),
			want: `{"id":0,"time":"0001-01-01 00:00:00","user":"","action":"","resource":"","details":"","ip":""}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := json.Marshal(c.got)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(b) != c.want {
				t.Errorf("JSON 形状变了\n got: %s\nwant: %s", string(b), c.want)
			}
		})
	}
}
