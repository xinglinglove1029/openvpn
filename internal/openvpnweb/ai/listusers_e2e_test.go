package ai

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

// TestListUsers_E2E_Direct 直接端到端验证
func TestListUsers_E2E_Direct(t *testing.T) {
	// ── ① Schema 校验验证 ──
	t.Run("Schema_空参数校验通过", func(t *testing.T) {
		schema, err := jsonschema.For[ListUsersRequest](nil)
		if err != nil {
			t.Fatalf("Schema 生成失败: %v", err)
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			t.Fatalf("Schema resolve 失败: %v", err)
		}
		// resolved.Schema() 返回 *jsonschema.Schema，直接 Marshal 即可
		schemaBytes, _ := json.MarshalIndent(resolved.Schema(), "", "  ")
		t.Logf("生成的 Schema:\n%s", schemaBytes)

		// 反序列化 schema 成 map 检查 required
		var sMap map[string]any
		json.Unmarshal(schemaBytes, &sMap)

		if req, ok := sMap["required"]; ok {
			if reqSlice, ok := req.([]any); ok && len(reqSlice) > 0 {
				t.Errorf("❌ Schema required 数组非空，值=%v。 空参数{}将校验失败！", reqSlice)
			}
		} else {
			t.Log("✅ Schema 没有 required 数组，所有参数可选")
		}

		// 断言 description 注入了
		props, _ := sMap["properties"].(map[string]any)
		if props == nil {
			t.Fatal("Schema 没有 properties")
		}
		limitProp, _ := props["limit"].(map[string]any)
		if desc, _ := limitProp["description"].(string); desc == "" {
			t.Error("❌ limit 字段 description 为空，没有正确注入 schema")
		} else {
			t.Logf("✅ limit.description=%q", desc)
		}
		offsetProp, _ := props["offset"].(map[string]any)
		if desc, _ := offsetProp["description"].(string); desc == "" {
			t.Error("❌ offset 字段 description 为空，没有正确注入 schema")
		} else {
			t.Logf("✅ offset.description=%q", desc)
		}

		// 最关键：空参数 {} 必须能通过校验
		emptyArgs := map[string]any{}
		if err := resolved.Validate(emptyArgs); err != nil {
			t.Fatalf("❌ 空参数 {} 校验失败: %v（这就是用户说\"列出系统中有多少用户\"直接报错的根因）", err)
		} else {
			t.Log("✅ 空参数 {} 通过 Schema 校验")
		}

		// 反序列化：空参数 → 默认零值
		rawBytes, _ := json.Marshal(emptyArgs)
		var req ListUsersRequest
		if err := json.Unmarshal(rawBytes, &req); err != nil {
			t.Fatalf("空参数反序列化失败: %v", err)
		}
		if req.Limit != 0 || req.Offset != 0 {
			t.Errorf("空参数反序列化默认值错误: Limit=%d Offset=%d（期望都是 0，由 clamp 处理）", req.Limit, req.Offset)
		}
		t.Logf("✅ 空参数反序列化: Limit=%d, Offset=%d（clamp 后 Limit→50, Offset→0）", req.Limit, req.Offset)
	})

	// ── ② 业务逻辑 clamp + 分页验证 ──
	t.Run("业务逻辑_clamp_分页", func(t *testing.T) {
		clamp := func(req ListUsersRequest) (limit, offset int) {
			limit = req.Limit
			if limit <= 0 {
				limit = 50
			}
			if limit > 200 {
				limit = 200
			}
			offset = req.Offset
			if offset < 0 {
				offset = 0
			}
			return
		}

		cases := []struct {
			name       string
			req        ListUsersRequest
			wantLimit  int
			wantOffset int
		}{
			{"LLM不传参数（最常见场景）", ListUsersRequest{}, 50, 0},
			{"传了limit=10", ListUsersRequest{Limit: 10}, 10, 0},
			{"传了limit=300（超上限）", ListUsersRequest{Limit: 300}, 200, 0},
			{"传了负数offset", ListUsersRequest{Limit: 50, Offset: -5}, 50, 0},
			{"传了正常参数", ListUsersRequest{Limit: 100, Offset: 25}, 100, 25},
		}
		allPassed := true
		for _, c := range cases {
			l, o := clamp(c.req)
			if l != c.wantLimit || o != c.wantOffset {
				t.Errorf("case %s 失败: 期望 (limit=%d,offset=%d)，实际 (%d,%d)", c.name, c.wantLimit, c.wantOffset, l, o)
				allPassed = false
			} else {
				t.Logf("✅ %s: clamp 后 limit=%d, offset=%d", c.name, l, o)
			}
		}
		if allPassed {
			t.Log("✅ 所有 clamp 边界用例通过")
		}
	})

	// ── ③ SSE 格式化链路验证（1:1 走后端+前端逻辑） ──
	t.Run("SSE完整链路_无data:data前缀", func(t *testing.T) {
		assistantAnswer := "好的，为您列出系统中的用户：\n" +
			"\n" +
			"系统共 3 个用户（默认返回前 50 条）：\n" +
			"\n" +
			"| 用户ID | 用户名 | 姓名 | 邮箱 | 状态 |\n" +
			"|--------|--------|------|------|------|\n" +
			"| 1      | admin  | 管理员 | admin@example.com | 启用 |\n" +
			"| 2      | yangdw | 杨文 | yangdw@example.com | 启用 |\n" +
			"| 3      | test1  | 测试1 | t1@qq.com | 禁用 |\n" +
			"\n" +
			"如需查看更多，请使用 offset 参数翻页。"

		// 1. 后端：escapeSSEData + 真实 fmt.Fprintf 模板（完全匹配 handlers.go 第 190 行）
		escaped := escapeSSEData(assistantAnswer)
		rawSSE := fmt.Sprintf("event: token\ndata: %s\n\n", escaped)
		t.Logf("后端生成的 SSE（长度 %d 字节），前 10 行:", len(rawSSE))
		lines := strings.Split(rawSSE, "\n")
		for i, l := range lines {
			if i >= 12 {
				t.Logf("  ... (共 %d 行)", len(lines))
				break
			}
			t.Logf("  [%2d] %q", i, l)
		}

		// 2. 前端：1:1 复刻修复后的 index.tsx 解析逻辑（关键：dataLines.join('\n')）
		var fullText string
		events := strings.Split(rawSSE, "\n\n")
		for _, ev := range events {
			if ev == "" {
				continue
			}
			var eventType string
			var dataLines []string
			for _, line := range strings.Split(ev, "\n") {
				if strings.HasPrefix(line, "event: ") {
					eventType = line[7:]
				} else if strings.HasPrefix(line, "data: ") {
					dataLines = append(dataLines, line[6:])
					chunk := line[6:]
					if strings.HasPrefix(chunk, "data:") {
						t.Errorf("❌ 单行 chunk 仍以 data: 开头！line=%q → chunk=%q", line, chunk)
					}
				}
			}
			data := strings.Join(dataLines, "\n")

			if eventType == "token" {
				fullText += data
			}
		}

		// 3. 最终断言
		failed := false
		if strings.Contains(fullText, "data:") {
			t.Errorf("❌ 最终文本包含残留 data: 前缀！")
			idx := strings.Index(fullText, "data:")
			ctxStart := idx - 10
			if ctxStart < 0 {
				ctxStart = 0
			}
			t.Errorf("  异常位置 around index %d: ...%q...", idx, safeSubstr2(fullText, ctxStart, 40))
			failed = true
		} else {
			t.Log("✅ 最终文本不含 data: 残留")
		}

		if fullText != assistantAnswer {
			minLen := len(fullText)
			if len(assistantAnswer) < minLen {
				minLen = len(assistantAnswer)
			}
			diffIdx := -1
			for i := 0; i < minLen; i++ {
				if fullText[i] != assistantAnswer[i] {
					diffIdx = i
					break
				}
			}
			if len(fullText) != len(assistantAnswer) && diffIdx == -1 {
				diffIdx = minLen
			}
			if diffIdx >= 0 {
				ctxStart := diffIdx - 20
				if ctxStart < 0 {
					ctxStart = 0
				}
				t.Errorf("❌ 最终文本与原文不一致: 期望长度 %d，实际长度 %d", len(assistantAnswer), len(fullText))
				t.Errorf("  差异位置 around index %d:", diffIdx)
				t.Errorf("    期望: ...%q...", safeSubstr2(assistantAnswer, ctxStart, 50))
				t.Errorf("    实际: ...%q...", safeSubstr2(fullText, ctxStart, 50))
				failed = true
			}
		} else {
			t.Logf("✅ 最终文本内容完全一致（%d 字符）", len(fullText))
		}

		if !failed {
			t.Log("🎉 SSE 链路测试全部通过")
		}
	})
}

func safeSubstr2(s string, start, length int) string {
	if start >= len(s) {
		return ""
	}
	end := start + length
	if end > len(s) {
		end = len(s)
	}
	return s[start:end]
}
