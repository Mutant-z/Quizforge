package parser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// 题目边界规则（docs/05 §8）：第一层规则，LLM 只处理不明确的。
var (
	reQuestionStart = regexp.MustCompile(`(?m)^\s*(?:(\d+)[\.、．]|第\s*\d+\s*题|[（(]\s*\d+\s*[）)])\s*`)
	reAnswerStart   = regexp.MustCompile(`(?m)^\s*(?:答案|参考答案)[:：]`)
	reAnalysisStart = regexp.MustCompile(`(?m)^\s*(?:解析|答案解析)[:：]`)
)

// IsQuestionStart 判断一行是否是题目开始。
func IsQuestionStart(line string) bool {
	return reQuestionStart.MatchString(line)
}

// QuestionNumber 提取题号（无则返回 0）。
func QuestionNumber(line string) int {
	m := reQuestionStart.FindStringSubmatch(line)
	if len(m) > 1 && m[1] != "" {
		var n int
		for _, ch := range m[1] {
			n = n*10 + int(ch-'0')
		}
		return n
	}
	return 0
}

// BuildCandidates 将分页文本流构建为候选题。
// 策略：按"题目开始"标记切分；跨页内容累积；答案/解析区（题本+答案混排）作为候选尾部保留。
func BuildCandidates(ctx context.Context, pages []Page) ([]*Candidate, error) {
	// 拼接为带页码标记的文本流
	type lineInfo struct {
		text   string
		pageNo int
	}
	stream := []lineInfo{}
	for _, p := range pages {
		if p.NeedsOCR || strings.TrimSpace(p.Text) == "" {
			continue // 扫描页跳过（需要 OCR 另行处理）
		}
		for _, line := range strings.Split(p.Text, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			stream = append(stream, lineInfo{text: line, pageNo: p.PageNo})
		}
	}

	var candidates []*Candidate
	var current *Candidate
	flush := func() {
		if current != nil && strings.TrimSpace(current.RawText) != "" {
			candidates = append(candidates, current)
		}
		current = nil
	}

	for _, li := range stream {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		line := strings.TrimSpace(li.text)
		if IsQuestionStart(line) {
			flush()
			current = &Candidate{
				PageStart: li.pageNo,
				PageEnd:   li.pageNo,
				RawText:   line,
			}
			continue
		}
		if current != nil {
			current.RawText += "\n" + line
			if li.pageNo > current.PageEnd {
				current.PageEnd = li.pageNo
			}
		}
	}
	flush()

	// 归一化 + hash
	out := make([]*Candidate, 0, len(candidates))
	for _, c := range candidates {
		c.RawText = strings.TrimSpace(c.RawText)
		c.NormalizedText = NormalizeText(c.RawText)
		sum := sha256.Sum256([]byte(c.NormalizedText))
		c.InputHash = hex.EncodeToString(sum[:16])
		c.Status = "pending"
		out = append(out, c)
	}
	return out, nil
}
