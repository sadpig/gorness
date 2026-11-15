package llm

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
)

// maxSSELineBytes 设定单行 64 MiB 安全上限，彻底消除 bufio.Scanner 64KB 截断导致的超大工具参数或 Base64 块报错。
const maxSSELineBytes = 64 << 20

var ErrSSELineTooLong = errors.New("sse: event line exceeds 64 MiB safety limit")

type SSEEvent struct {
	Event string
	Data  string
	ID    string
	Retry string
}

// SSEReader 基于无界行累加的 bufio.Reader 实现零依赖 SSE 帧解析器（同时兼容 Ollama 等网关的裸 NDJSON 行）。
type SSEReader struct {
	r      *bufio.Reader
	lastID string
}

func NewSSEReader(r io.Reader) *SSEReader {
	return &SSEReader{
		r: bufio.NewReaderSize(r, 64*1024),
	}
}

func (s *SSEReader) Next() (SSEEvent, error) {
	var (
		eventType string
		retryVal  string
		dataLines []string
		hasData   bool
	)

	for {
		line, err := s.readLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if hasData {
					return SSEEvent{
						Event: eventType,
						Data:  strings.Join(dataLines, "\n"),
						ID:    s.lastID,
						Retry: retryVal,
					}, nil
				}
				return SSEEvent{}, io.EOF
			}
			return SSEEvent{}, err
		}

		if line == "" {
			if hasData {
				return SSEEvent{
					Event: eventType,
					Data:  strings.Join(dataLines, "\n"),
					ID:    s.lastID,
					Retry: retryVal,
				}, nil
			}
			eventType = ""
			retryVal = ""
			dataLines = dataLines[:0]
			continue
		}

		if strings.HasPrefix(line, ":") {
			continue
		}

		// 兼容非 SSE 包装的裸 NDJSON 流式分块（WHATWG SSE 字段名绝不会以 '{' 开头）。
		trimmedLeft := strings.TrimLeft(line, " \t")
		if !hasData && eventType == "" && strings.HasPrefix(trimmedLeft, "{") {
			return SSEEvent{
				Data: strings.TrimSpace(line),
				ID:   s.lastID,
			}, nil
		}

		field, val, hasColon := strings.Cut(line, ":")
		if hasColon && len(val) > 0 && val[0] == ' ' {
			val = val[1:]
		}

		switch field {
		case "event":
			eventType = val
		case "data":
			dataLines = append(dataLines, val)
			hasData = true
		case "id":
			if !strings.ContainsRune(val, '\x00') {
				s.lastID = val
			}
		case "retry":
			retryVal = val
		}
	}
}

func (s *SSEReader) readLine() (string, error) {
	slice, err := s.r.ReadSlice('\n')
	if err == nil {
		return trimCRLF(slice), nil
	}
	if !errors.Is(err, bufio.ErrBufferFull) {
		if errors.Is(err, io.EOF) && len(slice) > 0 {
			return trimCRLF(slice), nil
		}
		return "", err
	}

	var buf bytes.Buffer
	if _, wErr := buf.Write(slice); wErr != nil {
		return "", wErr
	}

	for errors.Is(err, bufio.ErrBufferFull) {
		slice, err = s.r.ReadSlice('\n')
		if len(slice) > 0 {
			if buf.Len()+len(slice) > maxSSELineBytes {
				return "", fmt.Errorf("%w (%d bytes)", ErrSSELineTooLong, buf.Len()+len(slice))
			}
			_, _ = buf.Write(slice)
		}
	}

	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return trimCRLF(buf.Bytes()), nil
}

func trimCRLF(b []byte) string {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	if len(b) > 0 && b[len(b)-1] == '\r' {
		b = b[:len(b)-1]
	}
	return string(b)
}
