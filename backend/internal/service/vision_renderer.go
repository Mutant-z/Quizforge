package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/quiztrace/quiztrace/internal/domain"
)

const (
	directoryRenderVersion     = "vision-directory-150dpi-v1"
	extractionRenderVersion    = "vision-extraction-200dpi-v1"
	catalogRenderVersionV11    = "vision-catalog-135dpi-jpeg78-v2"
	extractionRenderVersionV11 = "vision-extraction-180dpi-jpeg88-v2"
	reviewRenderVersionV11     = "vision-review-2400px-png-v2"
)

type visionPageWindow struct {
	Index        int
	OwnedStart   int
	OwnedEnd     int
	ContextStart int
	ContextEnd   int
}

func buildVisionWindows(pageCount int) []visionPageWindow {
	const ownedPages = 4
	const overlapPages = 1
	out := []visionPageWindow{}
	for start, index := 1, 1; start <= pageCount; start, index = start+ownedPages, index+1 {
		ownedEnd := minInt(start+ownedPages-1, pageCount)
		contextEnd := minInt(ownedEnd+overlapPages, pageCount)
		out = append(out, visionPageWindow{Index: index, OwnedStart: start, OwnedEnd: ownedEnd, ContextStart: start, ContextEnd: contextEnd})
	}
	return out
}

func (s *ImportService) renderVisionPages(ctx context.Context, doc *domain.Document, pageCount int, pipelineVersion string, onProgress func(done, total int, purpose string, page int)) error {
	pdfToPPM, err := resolvePDFToPPM()
	if err != nil {
		return err
	}
	base := filepath.Join(s.cfg.Upload.Root, "page-assets", fmt.Sprintf("%d", doc.ID))
	type renderSpec struct {
		purpose, version      string
		dpi, maxEdge, quality int
		format, mimeType      string
	}
	specs := []renderSpec{
		{"directory", directoryRenderVersion, 150, 1800, 0, "png", "image/png"},
		{"extraction", extractionRenderVersion, 200, 2400, 0, "png", "image/png"},
	}
	if pipelineVersion == domain.VisionPipelineVersionV11 {
		specs = []renderSpec{
			{"catalog", catalogRenderVersionV11, 135, 1600, 78, "jpeg", "image/jpeg"},
			{"extraction", extractionRenderVersionV11, 180, 2200, 88, "jpeg", "image/jpeg"},
		}
	}
	tasks := make(chan struct {
		page int
		spec renderSpec
	})
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	var completed int64
	total := pageCount * len(specs)
	workers := 2
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				if err := s.renderVisionPage(ctx, pdfToPPM, doc, base, task.page, task.spec.purpose, task.spec.version, task.spec.dpi, task.spec.maxEdge, task.spec.format, task.spec.mimeType, task.spec.quality); err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
				done := int(atomic.AddInt64(&completed, 1))
				if onProgress != nil {
					onProgress(done, total, task.spec.purpose, task.page)
				}
			}
		}()
	}
	for _, spec := range specs {
		for page := 1; page <= pageCount; page++ {
			select {
			case tasks <- struct {
				page int
				spec renderSpec
			}{page, spec}:
			case err := <-errCh:
				close(tasks)
				wg.Wait()
				return err
			case <-ctx.Done():
				close(tasks)
				wg.Wait()
				return ctx.Err()
			}
		}
	}
	close(tasks)
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func (s *ImportService) renderVisionPage(ctx context.Context, pdfToPPM string, doc *domain.Document, base string, page int, purpose, version string, dpi, maxEdge int, format, mimeType string, quality int) error {
	dir := filepath.Join(base, purpose)
	if version == catalogRenderVersionV11 || version == extractionRenderVersionV11 || version == reviewRenderVersionV11 {
		dir = filepath.Join(base, version)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	prefix := filepath.Join(dir, fmt.Sprintf("page-%04d", page))
	extension := ".png"
	formatArg := "-png"
	if format == "jpeg" {
		extension = ".jpg"
		formatArg = "-jpeg"
	}
	output := prefix + extension
	if stat, err := os.Stat(output); err == nil && stat.Size() > 0 {
		return s.savePageAsset(ctx, doc.ID, page, purpose, version, output, mimeType)
	}
	args := []string{"-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-singlefile", formatArg, "-r", strconv.Itoa(dpi), "-scale-to", strconv.Itoa(maxEdge)}
	if format == "jpeg" && quality > 0 {
		args = append(args, "-jpegopt", fmt.Sprintf("quality=%d", quality))
	}
	args = append(args, doc.FilePath, prefix)
	cmd := exec.CommandContext(ctx, pdfToPPM, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("render page %d: %w: %s", page, err, string(out))
	}
	return s.savePageAsset(ctx, doc.ID, page, purpose, version, output, mimeType)
}

func (s *ImportService) ensureVisionReviewAssets(ctx context.Context, doc *domain.Document, start, end int) error {
	pdfToPPM, err := resolvePDFToPPM()
	if err != nil {
		return err
	}
	base := filepath.Join(s.cfg.Upload.Root, "page-assets", fmt.Sprintf("%d", doc.ID))
	for page := start; page <= end; page++ {
		if err := s.renderVisionPage(ctx, pdfToPPM, doc, base, page, "review", reviewRenderVersionV11, 220, 2400, "png", "image/png", 0); err != nil {
			return err
		}
	}
	return nil
}

func resolvePDFToPPM() (string, error) {
	candidates := []string{}
	if configured := os.Getenv("QT_PDFTOPPM_PATH"); configured != "" {
		candidates = append(candidates, configured)
	}
	if discovered, err := exec.LookPath("pdftoppm"); err == nil {
		candidates = append(candidates, discovered)
	}
	candidates = append(candidates,
		"/opt/homebrew/bin/pdftoppm",
		"/usr/local/bin/pdftoppm",
		"/opt/local/bin/pdftoppm",
	)
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
	return "", fmt.Errorf("缺少 PDF 页面渲染程序 pdftoppm：macOS 请执行 brew install poppler；容器请安装 poppler-utils；也可通过 QT_PDFTOPPM_PATH 指定可执行文件绝对路径")
}

func (s *ImportService) savePageAsset(ctx context.Context, documentID int64, page int, purpose, version, path, mimeType string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	config, _, err := image.DecodeConfig(f)
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	return s.repo.UpsertPageAsset(ctx, &domain.DocumentPageAsset{DocumentID: documentID, PageNo: page, RenderVersion: version, Purpose: purpose, FilePath: path, MIMEType: mimeType, Width: config.Width, Height: config.Height, SHA256: fmt.Sprintf("%x", h.Sum(nil)), Status: "ready"})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
