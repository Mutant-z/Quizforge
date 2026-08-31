package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/quiztrace/quiztrace/internal/domain"
	"github.com/quiztrace/quiztrace/internal/parser"
)

const ocrVersion = "tesseract.v1"

func (s *ImportService) runOCRForPages(ctx context.Context, doc *domain.Document, pages []parser.Page, renderPurpose, renderVersion string) error {
	if !s.cfg.Worker.OCREnabled {
		return nil
	}
	needsOCR := 0
	for _, page := range pages {
		if page.NeedsOCR {
			needsOCR++
		}
	}
	if needsOCR == 0 {
		return nil
	}
	tesseract, err := resolveTesseract(s.cfg.Worker.OCRBinaryPath)
	if err != nil {
		return err
	}
	for _, page := range pages {
		if !page.NeedsOCR {
			continue
		}
		asset, assetErr := s.repo.ListPageAssetsVersion(ctx, doc.ID, renderPurpose, renderVersion, page.PageNo, page.PageNo)
		if assetErr != nil || len(asset) != 1 {
			return fmt.Errorf("OCR_FAILED: page %d image unavailable", page.PageNo)
		}
		ocrCtx := ctx
		cancel := func() {}
		if s.cfg.Worker.OCRTimeoutSeconds > 0 {
			ocrCtx, cancel = context.WithTimeout(ctx, timeDurationSeconds(s.cfg.Worker.OCRTimeoutSeconds))
		}
		cmd := exec.CommandContext(ocrCtx, tesseract, "--psm", "6", "-l", s.cfg.Worker.OCRLanguage, asset[0].FilePath, "stdout")
		output, cmdErr := cmd.Output()
		cancel()
		if cmdErr != nil {
			_ = s.repo.UpdateDocumentPageOCR(ctx, doc.ID, int64(page.PageNo), "", "failed", "tesseract", ocrVersion, 0)
			return fmt.Errorf("OCR_FAILED: page %d: %w", page.PageNo, cmdErr)
		}
		text := parser.CleanText(string(output))
		if strings.TrimSpace(text) == "" {
			_ = s.repo.UpdateDocumentPageOCR(ctx, doc.ID, int64(page.PageNo), "", "failed", "tesseract", ocrVersion, 0)
			return fmt.Errorf("OCR_FAILED: page %d returned empty text", page.PageNo)
		}
		if err := s.repo.UpdateDocumentPageOCR(ctx, doc.ID, int64(page.PageNo), text, "success", "tesseract", ocrVersion, 0.75); err != nil {
			return err
		}
	}
	return nil
}

func resolveTesseract(configured string) (string, error) {
	candidates := []string{}
	if strings.TrimSpace(configured) != "" {
		candidates = append(candidates, configured)
	}
	if discovered, err := exec.LookPath("tesseract"); err == nil {
		candidates = append(candidates, discovered)
	}
	candidates = append(candidates, "/opt/homebrew/bin/tesseract", "/usr/local/bin/tesseract", "/usr/bin/tesseract")
	for _, candidate := range candidates {
		path, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("OCR_REQUIRED: 缺少 Tesseract，请安装并配置 QT_IMPORT_TESSERACT_PATH")
}

func timeDurationSeconds(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}

func (s *ImportService) ocrContextForWindow(ctx context.Context, documentID int64, start, end int) string {
	pages, err := s.repo.GetDocumentPages(ctx, documentID)
	if err != nil {
		return ""
	}
	var parts []string
	for _, page := range pages {
		if page.PageNo < start || page.PageNo > end || strings.TrimSpace(page.Text) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("OCR/文本层 物理页 %d：%s", page.PageNo, page.Text))
	}
	value := strings.Join(parts, "\n")
	if len(value) > 24000 {
		value = value[:24000] + "…"
	}
	return value
}
