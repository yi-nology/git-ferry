// AI 助手配置/对话 DTO(IDL 生成,与 API 字段对齐)。

struct AIConfigRequest {
    1: bool enabled (api.json="enabled")
    2: string baseUrl (api.json="base_url")
    3: string model (api.json="model")
    4: double temperature (api.json="temperature")
    5: i32 maxTokens (api.json="max_tokens")
    6: i32 timeoutSeconds (api.json="timeout_seconds")
    7: i32 maxConcurrentChats (api.json="max_concurrent_chats")
    8: optional string apiKey (api.json="api_key")
}

struct AITestRequest {
    1: string baseUrl (api.json="base_url")
    2: optional string model (api.json="model")
    3: optional string apiKey (api.json="api_key")
}

struct AIConfirmCall {
    1: string tool (api.json="tool")
    2: string token (api.json="token")
}

struct AIChatRequest {
    1: optional string sessionId (api.json="session_id")
    2: string message (api.json="message")
    3: optional AIConfirmCall confirmedToolCall (api.json="confirmed_tool_call")
}
