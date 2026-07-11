package server

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeFrame(t *testing.T) {
	tests := []struct {
		name string
		typ  byte
		data []byte
	}{
		{"output", FrameOutput, []byte("hello\x1b[0m")},
		{"input", FrameInput, []byte("a")},
		{"empty payload", FrameError, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeFrame(tt.typ, tt.data)
			typ, data, err := DecodeFrame(encoded)
			if err != nil {
				t.Fatalf("DecodeFrame: %v", err)
			}
			if typ != tt.typ {
				t.Fatalf("type: got %q want %q", typ, tt.typ)
			}
			if !bytes.Equal(data, tt.data) && !(len(data) == 0 && len(tt.data) == 0) {
				t.Fatalf("data: got %q want %q", data, tt.data)
			}
		})
	}
}

func TestDecodeFrame_EmptyMessageErrors(t *testing.T) {
	if _, _, err := DecodeFrame(nil); err == nil {
		t.Fatal("expected error on empty message")
	}
}

func TestEncodeDecodeResize(t *testing.T) {
	tests := []struct {
		cols, rows uint16
	}{
		{80, 24}, {0, 0}, {65535, 65535}, {64, 1},
	}
	for _, tt := range tests {
		encoded := EncodeResize(tt.cols, tt.rows)
		cols, rows, err := DecodeResize(encoded)
		if err != nil {
			t.Fatalf("DecodeResize: %v", err)
		}
		if cols != tt.cols || rows != tt.rows {
			t.Fatalf("got (%d,%d) want (%d,%d)", cols, rows, tt.cols, tt.rows)
		}
	}
}

func TestDecodeResize_WrongLengthErrors(t *testing.T) {
	for _, n := range []int{0, 1, 3, 5} {
		if _, _, err := DecodeResize(make([]byte, n)); err == nil {
			t.Fatalf("expected error for %d-byte resize payload", n)
		}
	}
}

func TestClampSize(t *testing.T) {
	tests := []struct {
		inCols, inRows     uint16
		wantCols, wantRows uint16
	}{
		{0, 0, minCols, minRows},
		{1, 100, minCols, 100},
		{100, 1, 100, minRows},
		{80, 24, 80, 24},
	}
	for _, tt := range tests {
		got := clampSize(tt.inCols, tt.inRows)
		if got.Cols != tt.wantCols || got.Rows != tt.wantRows {
			t.Fatalf("clampSize(%d,%d) = %+v, want cols=%d rows=%d", tt.inCols, tt.inRows, got, tt.wantCols, tt.wantRows)
		}
	}
}
