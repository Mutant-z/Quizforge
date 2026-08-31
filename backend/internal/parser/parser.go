package parser

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ParseDocument 解析文档为分页文本。
// 策略（docs/05）：
//   - PDF：优先文本层；文本密度过低的页面标记 NeedsOCR（OCR_REQUIRED）。
//   - TXT / MD：整文件作为单页。
//   - 其他：返回错误。
func ParseDocument(ctx context.Context, filePath, mimeType string) ([]Page, error) {
	switch {
	case strings.Contains(mimeType, "pdf"):
		return parsePDF(ctx, filePath)
	case strings.Contains(mimeType, "text/plain"), strings.Contains(mimeType, "markdown"), strings.Contains(mimeType, "text/markdown"):
		return parseTextFile(ctx, filePath)
	default:
		return nil, fmt.Errorf("unsupported mime type: %s", mimeType)
	}
}

func parseTextFile(ctx context.Context, filePath string) ([]Page, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read text file: %w", err)
	}
	return []Page{{PageNo: 1, Text: string(data), TextSource: "text_layer", OCRStatus: "not_required"}}, nil
}

// parsePDF 使用文本层解析，逐页提取。大文件按页读取，不整体载入内存。
func parsePDF(ctx context.Context, filePath string) ([]Page, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	reader, err := pdf.NewReader(f, stat.Size())
	if err != nil {
		return nil, fmt.Errorf("read pdf: %w", err)
	}

	pages := make([]Page, 0, reader.NumPage())
	for i := 1; i <= reader.NumPage(); i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		p := reader.Page(i)
		text, err := p.GetPlainText(nil)
		if err != nil {
			pages = append(pages, Page{PageNo: i, Text: "", NeedsOCR: true, TextSource: "none", OCRStatus: "required"})
			continue
		}
		text = CleanText(text)
		// 文本密度过低 → OCR 标记
		needsOCR := len(strings.TrimSpace(text)) < 20
		textSource, ocrStatus := "text_layer", "not_required"
		if needsOCR {
			textSource, ocrStatus = "none", "required"
		}
		pages = append(pages, Page{PageNo: i, Text: text, NeedsOCR: needsOCR, TextSource: textSource, OCRStatus: ocrStatus})
	}
	return pages, nil
}
