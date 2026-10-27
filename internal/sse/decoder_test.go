package sse

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestDecoderFraming(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n", "\r"} {
		t.Run(strings.ReplaceAll(ending, "\n", "LF"), func(t *testing.T) {
			body := strings.Join([]string{"\ufeff: comment", "id: ignored", "event: message", "data:first", "data: second", "", "data:", "", ""}, ending)
			d := NewDecoder(strings.NewReader(body), 0)
			e, err := d.Next()
			if err != nil || e.Name != "message" || e.Data != "first\nsecond" {
				t.Fatalf("bad framing: %+v %v", e, err)
			}
			e, err = d.Next()
			if err != nil || e.Data != "" {
				t.Fatalf("empty data event lost: %+v %v", e, err)
			}
			if _, err = d.Next(); !errors.Is(err, io.EOF) {
				t.Fatal(err)
			}
		})
	}
}
func TestDecoderBoundsAndIncompleteEvents(t *testing.T) {
	for _, body := range []string{"data:incomplete\n", "data:incomplete", "data:" + strings.Repeat("x", 80) + "\n\n", "data:" + strings.Repeat("x", 25) + "\ndata:" + strings.Repeat("x", 25) + "\n\n"} {
		if e, err := NewDecoder(strings.NewReader(body), 50).Next(); err == nil {
			t.Fatalf("partial or oversized event dispatched: %+v", e)
		}
	}
}

type oneByteReader struct{ io.Reader }

func (r oneByteReader) Read(p []byte) (int, error) { return r.Reader.Read(p[:1]) }

func TestDecoderHandlesCRLFSplitBetweenReads(t *testing.T) {
	e, err := NewDecoder(oneByteReader{strings.NewReader("data:small\r\n\r\n")}, 0).Next()
	if err != nil || e.Data != "small" {
		t.Fatalf("split CRLF changed the event: %+v %v", e, err)
	}
}
