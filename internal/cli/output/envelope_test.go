package output

import (
	"encoding/json"
	"testing"
)

func TestFromServer_StdShell(t *testing.T) {
	raw := json.RawMessage(`{"code":200,"message":"success","data":{"key":"t1"},"timestamp":1}`)
	env := FromServer(200, raw)
	if !env.OK {
		t.Fatalf("expected ok, got %+v", env)
	}
}

func TestFromServer_StdShellBizError(t *testing.T) {
	raw := json.RawMessage(`{"code":404,"message":"not found","data":null,"timestamp":1}`)
	env := FromServer(200, raw)
	if env.OK {
		t.Fatal("expected failure")
	}
	if env.Error.Code != 404 {
		t.Fatalf("code=%v", env.Error.Code)
	}
	if env.Error.Suggestion == "" {
		t.Fatal("expected suggestion")
	}
}

func TestFromServer_ListWrap(t *testing.T) {
	raw := json.RawMessage(`{"tasks":[{"key":"t1"}],"total":1}`)
	env := FromServer(200, raw)
	if !env.OK {
		t.Fatalf("expected ok, got %+v", env)
	}
	if env.Meta == nil || env.Meta.TotalCount != 1 {
		t.Fatalf("meta=%+v", env.Meta)
	}
	var arr []map[string]any
	if err := json.Unmarshal(env.Data.(json.RawMessage), &arr); err != nil || len(arr) != 1 {
		t.Fatalf("data=%v err=%v", env.Data, err)
	}
}

func TestFromServer_HTTPError(t *testing.T) {
	raw := json.RawMessage(`{"code":404,"message":"not found"}`)
	env := FromServer(404, raw)
	if env.OK {
		t.Fatal("expected failure")
	}
	if env.Error.Message != "not found" {
		t.Fatalf("message=%q", env.Error.Message)
	}
}

func TestFromServer_BareArray(t *testing.T) {
	env := FromServer(200, json.RawMessage(`[1,2,3]`))
	if !env.OK {
		t.Fatalf("expected ok, got %+v", env)
	}
}
