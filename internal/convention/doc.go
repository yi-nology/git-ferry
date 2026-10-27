// Package convention 承载**代码约定**的机械门禁。
//
// ## 为什么要有
//
// oekit 家族给每仓一条硬规则集(见仓根 AGENTS.md):
//
//  1. 错误处理必须用 pkg/errors(类型同一性红线:与 stdlib 同型);
//  2. 日志用 logx 的 kv 风格,包级 logger 用 Delegating;
//  3. 禁止裸 go func(用 concurrency/async 收编 panic);
//  4. SDK 包用 Delegating。
//
// 这些规则**靠人记必然衰减**:一次「顺手写个 go func」没人拦,半年后
// 就成了「这里为什么没日志」⇒ 排查成本全部转嫁给未来的人。
//
// 所以规则要落成**可执行的检查** —— 与 internal/release 的发版门禁同思路:
// 把纪律变成闸门,而不是变成文档里的一句话。
package convention
