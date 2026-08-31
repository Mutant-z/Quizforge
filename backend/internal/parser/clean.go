package parser

import (
	"regexp"
	"strings"
	"unicode"
)

// 清洗规则：页眉/页脚/页码/控制字符/重复空白（docs/05 §7）
var (
	rePageNumber   = regexp.MustCompile(`(?m)^\s*(?:第\s*\d+\s*页|[-—–]?\s*\d+\s*[-—–]?|Page\s*\d+)\s*$`)
	reControlChars = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
	reSpaceRun     = regexp.MustCompile(`[ \t]+`)
)

// CleanText 清洗单页文本。
func CleanText(text string) string {
	// 控制字符
	text = reControlChars.ReplaceAllString(text, "")
	// 行内多余空格
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		line = reSpaceRun.ReplaceAllString(line, " ")
		lines[i] = strings.TrimRight(line, " \t")
	}
	text = strings.Join(lines, "\n")
	// 页码行
	text = rePageNumber.ReplaceAllString(text, "")
	// 重复行（页眉等连续重复行，仅保留一次）
	text = dedupeConsecutiveLines(text)
	// 全角空格
	text = strings.ReplaceAll(text, "\u3000", " ")
	return text
}

// dedupeConsecutiveLines 去掉连续出现的相同行（页眉页脚特征）。
func dedupeConsecutiveLines(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if i > 0 && line == lines[i-1] && len(strings.TrimSpace(line)) >= 4 {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// NormalizeText 归一化（去空白、统一大小写）用于 hash 去重。
func NormalizeText(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			continue
		case unicode.IsUpper(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
