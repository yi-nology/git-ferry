// Package schema 从内嵌 OpenAPI 生成 API 自省视图，供 `gitferry schema` 使用。
package schema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/yi-nology/git-ferry/internal/pkg/swagger"
)

// Endpoint 是单个 API 端点的精简描述。
type Endpoint struct {
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Summary     string   `json:"summary,omitempty"`
	Domain      string   `json:"domain"`
	Params      []string `json:"params,omitempty"`
	HasBody     bool     `json:"has_body,omitempty"`
	OperationID string   `json:"operation_id,omitempty"`
}

// Domain 是按路径前缀分组的 API 域。
type Domain struct {
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	Endpoints []Endpoint `json:"endpoints"`
}

type openapiDoc struct {
	Paths map[string]map[string]struct {
		Summary     string `json:"summary"`
		OperationID string `json:"operationId"`
		Parameters  []struct {
			Name string `json:"name"`
			In   string `json:"in"`
		} `json:"parameters"`
		RequestBody *struct {
			Content map[string]json.RawMessage `json:"content"`
		} `json:"requestBody"`
	} `json:"paths"`
}

var (
	once    sync.Once
	domains []Domain
	loadErr error
)

// Load 解析内嵌 OpenAPI（幂等）。
func Load() ([]Domain, error) {
	once.Do(func() {
		domains, loadErr = parse(swagger.Spec())
	})
	return domains, loadErr
}

func parse(raw []byte) ([]Domain, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("openapi spec is empty")
	}
	var doc openapiDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse openapi: %w", err)
	}

	byDomain := map[string]*Domain{}
	prefixOf := func(path string) string {
		// /api/v1/sync/task → sync；/api/v1/ops/health → ops；/health → system
		p := strings.TrimPrefix(path, "/")
		parts := strings.Split(p, "/")
		if len(parts) >= 3 && parts[0] == "api" {
			return parts[2]
		}
		if len(parts) >= 1 {
			return parts[0]
		}
		return "other"
	}

	for path, ops := range doc.Paths {
		for method, op := range ops {
			method = strings.ToUpper(method)
			if method == "PARAMETERS" || method == "SUMMARY" {
				continue
			}
			domain := prefixOf(path)
			d := byDomain[domain]
			if d == nil {
				d = &Domain{Name: domain, Prefix: "/api/v1/" + domain}
				if domain == "system" || domain == "health" || domain == "ping" {
					d.Prefix = "/"
				}
				byDomain[domain] = d
			}
			var params []string
			for _, p := range op.Parameters {
				params = append(params, p.In+":"+p.Name)
			}
			d.Endpoints = append(d.Endpoints, Endpoint{
				Method:      method,
				Path:        path,
				Summary:     op.Summary,
				Domain:      domain,
				Params:      params,
				HasBody:     op.RequestBody != nil,
				OperationID: op.OperationID,
			})
		}
	}

	out := make([]Domain, 0, len(byDomain))
	for _, d := range byDomain {
		sort.Slice(d.Endpoints, func(i, j int) bool {
			if d.Endpoints[i].Path != d.Endpoints[j].Path {
				return d.Endpoints[i].Path < d.Endpoints[j].Path
			}
			return d.Endpoints[i].Method < d.Endpoints[j].Method
		})
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// List 返回全部域。
func List() ([]Domain, error) { return Load() }

// Show 返回某个域；name 可为域（sync）或 域.endpoint 模糊匹配（sync.task）。
func Show(name string) ([]Endpoint, error) {
	all, err := Load()
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	// 域级：sync
	for _, d := range all {
		if d.Name == name {
			return d.Endpoints, nil
		}
	}
	// 模糊：sync.task / task/run / /api/v1/sync/tasks
	q := strings.ToLower(name)
	var hits []Endpoint
	for _, d := range all {
		for _, e := range d.Endpoints {
			blob := strings.ToLower(e.Method + " " + e.Path + " " + e.Summary + " " + e.OperationID)
			if strings.Contains(blob, q) {
				hits = append(hits, e)
			}
		}
	}
	if len(hits) == 0 {
		return nil, fmt.Errorf("no API matched %q (try `gitferry schema list`)", name)
	}
	return hits, nil
}

// Find 返回 method+path 精确匹配（供 raw api 使用时校验）。
func Find(method, path string) (Endpoint, bool) {
	all, err := Load()
	if err != nil {
		return Endpoint{}, false
	}
	method = strings.ToUpper(method)
	for _, d := range all {
		for _, e := range d.Endpoints {
			if e.Method == method && e.Path == path {
				return e, true
			}
		}
	}
	return Endpoint{}, false
}
