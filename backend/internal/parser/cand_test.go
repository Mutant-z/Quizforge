package parser

import (
	"context"
	"os"
	"testing"
)

func TestParseAndBuildCandidates(t *testing.T) {
	// 生成文本型 PDF
	content := `第一章 基础知识
1. 下列哪种数据结构是先进后出？
A. 队列
B. 栈
C. 数组
D. 链表
答案：B
解析：栈遵循 LIFO 原则。

2. 下列属于排序算法的有？（多选）
A. 快速排序
B. 冒泡排序
C. 深度优先搜索
D. 归并排序
答案：A、B、D
解析：DFS 是图遍历算法。

3. HTTP 属于应用层协议。
A. 正确
B. 错误
答案：A
解析：HTTP 位于应用层。
`
	// 用文本文件模拟（避免依赖 PDF 生成库）
	f, err := os.CreateTemp("", "cand*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	pages, err := ParseDocument(context.Background(), f.Name(), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("want 1 page, got %d", len(pages))
	}
	cands, err := BuildCandidates(context.Background(), pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 3 {
		t.Fatalf("want 3 candidates, got %d", len(cands))
	}
	if cands[0].PageStart != 1 {
		t.Fatalf("page start mismatch: %d", cands[0].PageStart)
	}
	if len(cands[0].InputHash) != 32 {
		t.Fatalf("hash mismatch: %s", cands[0].InputHash)
	}
	if !containsStr(cands[0].RawText, "栈") {
		t.Fatalf("candidate text wrong: %s", cands[0].RawText)
	}
	t.Logf("OK: %d candidates, first=%q", len(cands), cands[0].RawText[:50])
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
