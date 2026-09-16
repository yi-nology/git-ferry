// aistub 是 OpenAI /chat/completions 兼容的本地桩,用于 AI 助手链路实测:
// 按用户消息关键词脚本化返回「先调工具 → 再总结」两轮行为。
// 仅本地/CI 实测用,不进部署镜像。
//
//	go run ./cmd/aistub -port 8899
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ===== OpenAI wire 结构(仅桩所需子集) =====

type chatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role       string `json:"role"`
		Content    string `json:"content"`
		ToolCallID string `json:"tool_call_id"`
	} `json:"messages"`
	Stream bool `json:"stream"`
}

type wireFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireToolCall struct {
	Index    int          `json:"index"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type chunkDelta struct {
	Role      string         `json:"role,omitempty"`
	Content   string         `json:"content,omitempty"`
	ToolCalls []wireToolCall `json:"tool_calls,omitempty"`
}

type chunkChoice struct {
	Index        int        `json:"index"`
	Delta        chunkDelta `json:"delta"`
	FinishReason string     `json:"finish_reason"`
}

type chunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []chunkChoice `json:"choices"`
}

// ===== 脚本 =====

type turn struct {
	toolName string
	toolArgs string
	text     string
}

// script 按最后一条 user 消息关键词决定两轮脚本。
func script(lastUser string) []turn {
	switch {
	case strings.Contains(lastUser, "仓库"):
		return []turn{
			{toolName: "list_repos", toolArgs: `{}`},
			{text: "已为你列出当前同步仓库,详见上方结果,均为 active 状态。"},
		}
	case strings.Contains(lastUser, "同步") || strings.Contains(lastUser, "触发") || strings.Contains(lastUser, "执行"):
		return []turn{
			{toolName: "run_task", toolArgs: `{"task_key":"demo-task"}`},
			{text: "该操作需要确认,已在界面上弹出确认卡片,确认后将立即执行同步。"},
		}
	default:
		return []turn{
			{toolName: "get_system_overview", toolArgs: `{}`},
			{text: "系统整体运行正常,仓库与任务状态如上,如需进一步操作请告诉我。"},
		}
	}
}

// ===== 处理 =====

func writeChunk(w io.Writer, model, role, content string, calls []wireToolCall, finish string) {
	c := chunk{
		ID:      fmt.Sprintf("chatcmpl-stub-%d", time.Now().UnixNano()),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []chunkChoice{{Index: 0, Delta: chunkDelta{Role: role, Content: content, ToolCalls: calls}, FinishReason: finish}},
	}
	b, _ := json.Marshal(c)
	fmt.Fprintf(w, "data: %s\n\n", b)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	var req chatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	_ = flusher

	// 找最后一条 user 消息;若请求里已有 tool 结果 → 输出第二轮文本
	lastUser := ""
	hasToolResult := false
	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			lastUser = m.Content
		case "tool":
			hasToolResult = true
		}
	}

	turns := script(lastUser)
	if hasToolResult {
		// 第二轮:分块输出总结文本
		text := "。" // 占位:脚本第二条
		if len(turns) > 1 {
			text = turns[1].text
		}
		writeChunk(w, req.Model, "assistant", "", nil, "")
		runes := []rune(text)
		for i := 0; i < len(runes); i += 4 {
			end := i + 4
			if end > len(runes) {
				end = len(runes)
			}
			writeChunk(w, req.Model, "assistant", string(runes[i:end]), nil, "")
		}
		writeChunk(w, req.Model, "", "", nil, "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}

	// 第一轮:发起工具调用
	t := turns[0]
	if t.toolName == "" {
		// 无工具脚本:直接输出文本
		writeChunk(w, req.Model, "assistant", t.text, nil, "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}
	call := wireToolCall{Index: 0, ID: fmt.Sprintf("call-%d", time.Now().UnixNano()), Type: "function",
		Function: wireFunction{Name: t.toolName, Arguments: t.toolArgs}}
	writeChunk(w, req.Model, "assistant", "", []wireToolCall{call}, "tool_calls")
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func main() {
	port := flag.Int("port", 8899, "监听端口")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", handleChat)
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":[{"id":"stub","object":"model"}]}`)
	})
	log.Printf("aistub listening on 127.0.0.1:%d", *port)
	srv := &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", *port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      120 * time.Second, // SSE 流式输出,放宽写超时
		IdleTimeout:       60 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
