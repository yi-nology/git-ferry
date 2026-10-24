// OpenAPI schema 构造小工具 —— 供本包内各「非 IDL 定义端点」的契约文件共用。
//
// 单独成文件是为了不绑死在某一个契约文件上:
// histsync.go / mirror.go 等都从这里取构造函数,谁都不必"拥有"它们。
package main

func schObj(props map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": props}
}

func schStr() map[string]any  { return map[string]any{"type": "string"} }
func schBool() map[string]any { return map[string]any{"type": "boolean"} }
func schInt64() map[string]any {
	return map[string]any{"type": "integer", "format": "int64"}
}
func schArr(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}
func schRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}
func schStrMap() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}
}
