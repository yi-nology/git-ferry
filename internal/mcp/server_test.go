package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
)

func newTestRegistry() *tools.Registry {
	return tools.NewRegistry(toolstest.NewMock())
}

func doRPC(t *testing.T, reg *tools.Registry, payload string) jsonrpcResp {
	t.Helper()
	body := app.NewContext(0)
	body.Request.SetBody([]byte(payload))
	body.Request.Header.SetMethod(consts.MethodPost)
	Handler(reg)(context.Background(), body)
	raw := body.Response.Body()
	var resp jsonrpcResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return resp
}

func TestMCP_Initialize(t *testing.T) {
	resp := doRPC(t, newTestRegistry(), `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if resp.Error != nil {
		t.Fatal(resp.Error)
	}
	b, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(b), "protocolVersion") {
		t.Fatalf("result=%s", b)
	}
}

func TestMCP_ToolsList(t *testing.T) {
	resp := doRPC(t, newTestRegistry(), `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if resp.Error != nil {
		t.Fatal(resp.Error)
	}
	b, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(b), "list_repos") {
		t.Fatalf("tools missing list_repos: %s", b)
	}
}

func TestMCP_ToolsCall(t *testing.T) {
	reg := newTestRegistry()
	resp := doRPC(t, reg, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_repos","arguments":{}}}`)
	if resp.Error != nil {
		t.Fatal(resp.Error)
	}
	b, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(b), "content") {
		t.Fatalf("result=%s", b)
	}
}

func TestMCP_UnknownMethod(t *testing.T) {
	resp := doRPC(t, newTestRegistry(), `{"jsonrpc":"2.0","id":4,"method":"nope"}`)
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("want method not found, got %+v", resp)
	}
}

func TestStatus(t *testing.T) {
	c := app.NewContext(0)
	Status(newTestRegistry())(context.Background(), c)
	if c.Response.StatusCode() != http.StatusOK {
		t.Fatalf("status=%d", c.Response.StatusCode())
	}
}
