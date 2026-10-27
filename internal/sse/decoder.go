// Package sse frames bounded server-sent events without interpreting their data.
package sse

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

const DefaultMaxEventBytes = 1 << 20

type Event struct{ Name, Data string }
type Decoder struct {
	scanner *bufio.Scanner
	limit   int
	first   bool
}

func NewDecoder(reader io.Reader, limit int) *Decoder {
	if limit <= 0 {
		limit = DefaultMaxEventBytes
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), limit+2)
	scanner.Split(splitLine)
	return &Decoder{scanner: scanner, limit: limit, first: true}
}

// splitLine handles LF, CRLF, and CR, including CRLF split between reads.
func splitLine(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' {
			return i + 1, data[:i], nil
		}
		if b == '\r' {
			if i+1 == len(data) && !atEOF {
				return 0, nil, nil
			}
			n := 1
			if i+1 < len(data) && data[i+1] == '\n' {
				n = 2
			}
			return i + n, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (d *Decoder) Next() (Event, error) {
	var event Event
	var data strings.Builder
	hasData := false
	size := 0
	for d.scanner.Scan() {
		line := d.scanner.Text()
		if d.first {
			line = strings.TrimPrefix(line, "\ufeff")
			d.first = false
		}
		if line == "" {
			if hasData {
				event.Data = strings.TrimSuffix(data.String(), "\n")
				return event, nil
			}
			event = Event{}
			size = 0
			continue
		}
		size += len(line) + 1
		if size > d.limit {
			return Event{}, fmt.Errorf("SSE event exceeds %d bytes", d.limit)
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			value = ""
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event.Name = value
		case "data":
			hasData = true
			data.WriteString(value)
			data.WriteByte('\n')
		}
	}
	if err := d.scanner.Err(); err != nil {
		return Event{}, err
	}
	// Pending data is never dispatched without its terminating blank line.
	if hasData {
		return Event{}, io.ErrUnexpectedEOF
	}
	return Event{}, io.EOF
}
