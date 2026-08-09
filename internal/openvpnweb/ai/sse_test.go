package ai

import "testing"

// TestEscapeSSEData 验证 escapeSSEData 不会产生重复的 data: 前缀
// Bug 场景：之前实现每行都会加 data: 前缀，而调用点 fmt.Fprintf("event: token\ndata: %s\n\n", escapeSSEData(x))
// 又写了一次 data:，导致每行都变成 data: data: xxx，前端解析时从第7个字符切片后剩下 "data: xxx"，反复拼接出 data:data:data:
func TestEscapeSSEData(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"单行无换行", "你好", "你好"},
		{"两行", "第一行\n第二行", "第一行\ndata: 第二行"},
		{"三行", "line1\nline2\nline3", "line1\ndata: line2\ndata: line3"},
		{"空字符串", "", ""},
		{"仅一个换行", "a\n", "a\ndata: "},
		{"中文换行", "用户列表：\n1. admin 管理员\n2. test 测试", "用户列表：\ndata: 1. admin 管理员\ndata: 2. test 测试"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeSSEData(tt.input)
			if got != tt.expected {
				t.Errorf("escapeSSEData(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}

	// 额外验证：拼到真实 fmt.Fprintf 格式中后，每一行开头只能有一个 data:
	t.Run("no_duplicate_prefix", func(t *testing.T) {
		text := "第一行\n第二行\n第三行"
		escaped := escapeSSEData(text)
		// 模拟后端真实输出
		actualOutput := fmtSprintf("event: token\ndata: %s\n\n", escaped)
		// 每行检查：第一行 data: 后不能再以 "data:" 开头
		lines := splitLines(actualOutput)
		for i, line := range lines {
			if line == "" {
				continue
			}
			if startsWith(line, "data: ") {
				afterPrefix := line[6:]
				if startsWith(afterPrefix, "data:") {
					t.Errorf("第 %d 行检测到重复 data: 前缀: %q", i, line)
				}
			}
		}
		// 断言至少有 3 个 data: 行（3 行内容 → 3 个 data: 前缀）
		dataLineCount := 0
		for _, line := range lines {
			if startsWith(line, "data: ") {
				dataLineCount++
			}
		}
		if dataLineCount != 3 {
			t.Errorf("期望 3 个 data: 行，实际 %d 个。输出:\n%s", dataLineCount, actualOutput)
		}
	})
}

// 辅助函数（避免引入额外依赖）
func fmtSprintf(format, s string) string {
	// 简单手动拼接，等同于 fmt.Sprintf("event: token\ndata: %s\n\n", s)
	return format[0:len("event: token\n")] + "data: " + s + "\n\n"
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[0:len(prefix)] == prefix
}
