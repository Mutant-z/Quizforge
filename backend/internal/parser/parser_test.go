package parser

import (
	"context"
	"strings"
	"testing"
)

func TestCleanText(t *testing.T) {
	input := "第 3 页\n页眉标题\n页眉标题\n问题正文  有多个空格\n"
	out := CleanText(input)
	if strings.Contains(out, "第 3 页") {
		t.Fatal("page number should be removed")
	}
	if strings.Contains(out, "问题正文  有") {
		t.Fatal("double spaces should be collapsed")
	}
}

func TestNormalizeText(t *testing.T) {
	a := NormalizeText("Hello World")
	b := NormalizeText("hello world")
	if a != b {
		t.Fatalf("normalize should ignore case: %q vs %q", a, b)
	}
}

func TestBuildCandidatesAcrossPages(t *testing.T) {
	// 跨页题目：题 1 在第 1 页开始，答案在第 2 页
	pages := []Page{
		{PageNo: 1, Text: "1. 跨页题目？\nA. 选项1\nB. 选项2\n答案：B\n解析：跨页解析"},
		{PageNo: 2, Text: "2. 第二题\nA. x\nB. y\n答案：A"},
	}
	cands, err := BuildCandidates(context.Background(), pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(cands))
	}
	if cands[0].PageStart != 1 || cands[0].PageEnd != 1 {
		t.Fatalf("page range wrong: %d-%d", cands[0].PageStart, cands[0].PageEnd)
	}
	if cands[1].PageStart != 2 {
		t.Fatalf("second candidate should start at page 2, got %d", cands[1].PageStart)
	}
}
