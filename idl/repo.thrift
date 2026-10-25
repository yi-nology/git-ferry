

struct RepoInfo {
    1: i64 id (api.json="id")
    2: string key (api.json="key")
    3: string name (api.json="name")
    4: string platform (api.json="platform")
    5: string platformOwner (api.json="platform_owner")
    6: string platformRepo (api.json="platform_repo")
    7: string cloneUrl (api.json="clone_url")
    8: string sshUrl (api.json="ssh_url")
    9: string defaultBranch (api.json="default_branch")
    10: string status (api.json="status")
    11: string createdAt (api.json="created_at")
}

struct ListReposReq {
    1: i32 page (api.query="page")
    2: i32 pageSize (api.query="page_size")
    3: string search (api.query="search")
    4: string platform (api.query="platform")
    5: string status (api.query="status")
    6: string sortBy (api.query="sort_by")
    7: string sortOrder (api.query="sort_order")
}

struct ListReposResp {
    1: list<RepoInfo> repos (api.json="repos")
    2: i64 total (api.json="total")
}

struct GetRepoReq {
    1: string key (api.query="key")
}

struct GetRepoResp {
    1: RepoInfo repo (api.json="repo")
}

struct CreateRepoReq {
    1: string name (api.json="name")
    2: string remoteUrl (api.json="remote_url")
}

struct CreateRepoResp {
    1: RepoInfo repo (api.json="repo")
}

struct UpdateRepoReq {
    1: string key (api.json="key")
    2: string name (api.json="name")
}

struct UpdateRepoResp {
    1: RepoInfo repo (api.json="repo")
}

struct DeleteRepoReq {
    1: string key (api.query="key")
}

struct DeleteRepoResp {
    1: bool success (api.json="success")
}

struct TestConnectionReq {
    1: string key (api.query="key")
}

struct TestConnectionResp {
    1: bool success (api.json="success")
    2: string message (api.json="message")
}

struct ListBranchesReq {
    1: string key (api.query="key")
}

struct ListBranchesResp {
    1: list<string> branches (api.json="branches")
}

struct BatchReposReq {
    1: string action (api.json="action")
    2: list<string> keys (api.json="keys")
}

struct BatchReposResp {
    1: i32 total (api.json="total")
    2: i32 success (api.json="success")
    3: i32 failed (api.json="failed")
    4: list<string> errors (api.json="errors")
}
