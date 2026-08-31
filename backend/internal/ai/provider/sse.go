package provider

import (
	"bufio"
	"io"
	"strings"
)

// sseReader 解析 SSE 流（兼容 OpenAI 与 Anthropic 的 data: 行）。
type sseReader struct {
	scanner *bufio.Scanner
}

func newSSEReader(r io.Reader) *sseReader {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &sseReader{scanner: sc}
}

// Next 返回下一个事件名与数据。
func (s *sseReader) Next() (event, data string, ok bool) {
	event = "message"
	data = ""
	for s.scanner.Scan() {
		line := strings.TrimRight(s.scanner.Text(), "\r")
		if line == "" {
			if data != "" {
				return event, data, true
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			d := strings.TrimPrefix(line, "data:")
			d = strings.TrimPrefix(d, " ")
			if data != "" {
				data += "\n"
			}
			data += d
			continue
		}
	}
	if data != "" {
		return event, data, true
	}
	return "", "", false
}
