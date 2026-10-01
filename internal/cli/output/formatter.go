package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

// Print 按 format 输出 Envelope：json（默认，Agent 用）/ table / yaml。
func Print(w io.Writer, env *Envelope, format string) error {
	switch format {
	case "", "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(env)
	case "yaml", "yml":
		b, err := yaml.Marshal(env)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	case "table":
		return printTable(w, env)
	default:
		return fmt.Errorf("unsupported format %q (use json|table|yaml)", format)
	}
}

func printTable(w io.Writer, env *Envelope) error {
	if !env.OK {
		if _, err := fmt.Fprintf(w, "ERROR [%v] %s\n", env.Error.Code, env.Error.Message); err != nil {
			return err
		}
		if env.Error.Suggestion != "" {
			if _, err := fmt.Fprintf(w, "  suggestion: %s\n", env.Error.Suggestion); err != nil {
				return err
			}
		}
		return nil
	}
	if env.Meta != nil && (env.Meta.TotalCount > 0 || env.Meta.Page > 0) {
		if _, err := fmt.Fprintf(w, "# page=%d limit=%d total=%d\n", env.Meta.Page, env.Meta.Limit, env.Meta.TotalCount); err != nil {
			return err
		}
	}
	return renderValue(w, env.Data)
}

func renderValue(w io.Writer, data interface{}) error {
	// json.RawMessage 先解开再渲染
	if raw, ok := data.(json.RawMessage); ok {
		var v interface{}
		if err := json.Unmarshal(raw, &v); err == nil {
			return renderValue(w, v)
		}
		_, err := fmt.Fprintln(w, string(raw))
		return err
	}
	switch v := data.(type) {
	case nil:
		_, err := fmt.Fprintln(w, "(empty)")
		return err
	case []interface{}:
		if len(v) == 0 {
			_, err := fmt.Fprintln(w, "(empty)")
			return err
		}
		if _, ok := v[0].(map[string]interface{}); ok {
			return renderObjects(w, v)
		}
		for _, item := range v {
			if _, err := fmt.Fprintln(w, item); err != nil {
				return err
			}
		}
		return nil
	case map[string]interface{}:
		return renderObjects(w, []interface{}{v})
	default:
		// 其他切片/结构（如 []map[string]any）→ 归一成 []interface{} / map 再渲染
		if b, err := json.Marshal(data); err == nil {
			var v interface{}
			if json.Unmarshal(b, &v) == nil {
				switch tv := v.(type) {
				case []interface{}:
					if len(tv) == 0 {
						_, err := fmt.Fprintln(w, "(empty)")
						return err
					}
					if _, ok := tv[0].(map[string]interface{}); ok {
						return renderObjects(w, tv)
					}
					b2, _ := json.MarshalIndent(tv, "", "  ")
					_, err := fmt.Fprintln(w, string(b2))
					return err
				case map[string]interface{}:
					return renderObjects(w, []interface{}{tv})
				}
			}
		}
		b, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(b))
		return err
	}
}

func renderObjects(w io.Writer, rows []interface{}) error {
	// 先收集全部列，再按 preferred 排序；只输出数据里真实存在的列
	colSet := map[string]bool{}
	var cols []string
	addCol := func(c string) {
		if c != "" && !colSet[c] {
			colSet[c] = true
			cols = append(cols, c)
		}
	}
	for _, r := range rows {
		m, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		for k := range m {
			addCol(k)
		}
	}
	preferred := []string{
		"key", "id", "name", "status", "task_key", "platform", "level", "score",
		"domain", "prefix", "endpoints", "method", "path", "summary", "message",
	}
	var ordered []string
	seen := map[string]bool{}
	for _, p := range preferred {
		if colSet[p] {
			ordered = append(ordered, p)
			seen[p] = true
		}
	}
	for _, c := range cols {
		if !seen[c] {
			ordered = append(ordered, c)
		}
	}
	cols = ordered
	if len(cols) > 8 {
		cols = cols[:8]
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(cols, "\t")); err != nil {
		return err
	}
	for _, r := range rows {
		m, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		vals := make([]string, 0, len(cols))
		for _, c := range cols {
			vals = append(vals, formatCell(m[c]))
		}
		if _, err := fmt.Fprintln(tw, strings.Join(vals, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func formatCell(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		if len(t) > 48 {
			return t[:45] + "..."
		}
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		s := string(b)
		if len(s) > 48 {
			return s[:45] + "..."
		}
		return s
	}
}
