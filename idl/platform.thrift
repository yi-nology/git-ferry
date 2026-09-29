// Platform management IDL. Req fields mirror the existing handler request
// structs (platform_service.go). Method names match the existing handler
// functions so hz preserves their business logic on regenerate.

struct PlatformInfo {
    1: i64 id (api.json="id")
    2: string key (api.json="key")
    3: string name (api.json="name")
    4: string type (api.json="type")
    5: string instanceUrl (api.json="instance_url")
    6: string apiUrl (api.json="api_url")
    7: bool skipTlsVerify (api.json="skip_tls_verify")
    8: string caCertPath (api.json="ca_cert_path")
    9: string proxyUrl (api.json="proxy_url")
    10: bool isDefault (api.json="is_default")
    11: string status (api.json="status")
    12: string lastTestResult (api.json="last_test_result")
    13: i32 repoCount (api.json="repo_count")
    14: string createdAt (api.json="created_at")
    15: string lastTestAt (api.json="last_test_at")
    16: string updatedAt (api.json="updated_at")
    17: bool hasToken (api.json="has_token")
    18: bool hasGithubApp (api.json="has_github_app")
    19: optional i64 githubAppId (api.json="github_app_id")
}

struct ListPlatformsReq {}

struct ListPlatformsResp {
    1: list<PlatformInfo> platforms (api.json="platforms")
    2: i64 total (api.json="total")
}

struct GetPlatformReq {
    1: string key (api.query="key")
}

struct GetPlatformResp {
    1: PlatformInfo platform (api.json="platform")
}

struct CreatePlatformReq {
    1: string name (api.json="name")
    2: string type (api.json="type")
    3: string instanceUrl (api.json="instance_url")
    4: string apiUrl (api.json="api_url")
    5: string accessToken (api.json="access_token")
    6: bool skipTlsVerify (api.json="skip_tls_verify")
    7: string caCertPath (api.json="ca_cert_path")
    8: string proxyUrl (api.json="proxy_url")
    9: bool isDefault (api.json="is_default")
    22: optional i64 githubAppId (api.json="github_app_id")
    23: optional i64 githubInstallationId (api.json="github_installation_id")
    24: optional string githubPrivateKey (api.json="github_private_key")
}

struct CreatePlatformResp {
    1: PlatformInfo platform (api.json="platform")
}

struct UpdatePlatformReq {
    1: string key (api.json="key")
    2: string name (api.json="name")
    3: string instanceUrl (api.json="instance_url")
    4: string apiUrl (api.json="api_url")
    5: string accessToken (api.json="access_token")
    6: optional bool skipTlsVerify (api.json="skip_tls_verify")
    7: string caCertPath (api.json="ca_cert_path")
    8: string proxyUrl (api.json="proxy_url")
    9: optional bool isDefault (api.json="is_default")
    22: optional i64 githubAppId (api.json="github_app_id")
    23: optional i64 githubInstallationId (api.json="github_installation_id")
    24: optional string githubPrivateKey (api.json="github_private_key")
}

struct UpdatePlatformResp {
    1: PlatformInfo platform (api.json="platform")
}

struct DeletePlatformReq {
    1: string key (api.query="key")
}

struct DeletePlatformResp {
    1: bool success (api.json="success")
}

struct SetDefaultPlatformReq {
    1: string key (api.query="key")
}

struct SetDefaultPlatformResp {
    1: string message (api.json="message")
}

struct TestPlatformConnectionReq {
    1: string key (api.query="key")
}

struct TestPlatformConnectionResp {
    1: string result (api.json="result")
}

struct ListPlatformReposReq {
    1: string key (api.query="key")
    2: string page (api.query="page")
    3: string perPage (api.query="per_page")
}

struct ListPlatformReposResp {
    1: i32 total (api.json="total")
}

struct SyncPlatformReposReq {
    1: string key (api.query="key")
}

struct SyncPlatformReposResp {
    1: string message (api.json="message")
    2: i32 syncedCount (api.json="synced_count")
}
