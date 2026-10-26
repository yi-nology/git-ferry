#!/usr/bin/env python3
"""假 ACP agent:按 ACP 协议应答,用于 stdio transport 的确定性测试。

真实 agent 依赖模型与网络,测不了「协议层到底通没通」;
这个脚本把协议走完(initialize → session/new → session/prompt → session/update),
让测试只依赖协议本身。
"""
import json
import sys


def send(obj):
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()


for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        req = json.loads(line)
    except Exception:
        continue
    method = req.get("method")
    rid = req.get("id")
    if method == "initialize":
        send({
            "jsonrpc": "2.0",
            "id": rid,
            "result": {
                "protocolVersion": 1,
                "agentCapabilities": {
                    "loadSession": True,
                    "mcpCapabilities": {"http": True, "sse": True},
                    "promptCapabilities": {"image": False, "audio": False},
                },
                "agentInfo": {"name": "fake-agent", "title": "Fake ACP Agent", "version": "9.9"},
            },
        })
    elif method == "session/new":
        send({"jsonrpc": "2.0", "id": rid, "result": {"sessionId": "fake-acp-session-1"}})
    elif method == "session/prompt":
        sid = (req.get("params") or {}).get("sessionId")
        send({
            "jsonrpc": "2.0",
            "method": "session/update",
            "params": {
                "sessionId": sid,
                "update": {
                    "sessionUpdate": "agent_message_chunk",
                    "messageId": "m1",
                    "content": {"type": "text", "text": "你好，我是假 agent"},
                },
            },
        })
        send({"jsonrpc": "2.0", "id": rid, "result": {"stopReason": "end_turn"}})
    elif rid is not None:
        # 其它请求(权限/fs/terminal)一律空应答
        send({"jsonrpc": "2.0", "id": rid, "result": {}})
