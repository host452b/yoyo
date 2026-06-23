package proxy

import (
	"bytes"
	"testing"
)

func TestNormalizeKittyPrefix(t *testing.T) {
	const ctrlY = byte(0x19)

	tests := []struct {
		name string
		in   []byte
		want []byte
	}{
		{
			name: "legacy ctrl+y passes through unchanged",
			in:   []byte{ctrlY},
			want: []byte{ctrlY},
		},
		{
			name: "kitty ctrl+y press becomes legacy byte",
			in:   []byte("\x1b[121;5u"),
			want: []byte{ctrlY},
		},
		{
			name: "kitty ctrl+y with explicit press event",
			in:   []byte("\x1b[121;5:1u"),
			want: []byte{ctrlY},
		},
		{
			name: "kitty ctrl+y repeat event becomes legacy byte",
			in:   []byte("\x1b[121;5:2u"),
			want: []byte{ctrlY},
		},
		{
			name: "kitty ctrl+y release event is dropped",
			in:   []byte("\x1b[121;5:3u"),
			want: []byte{},
		},
		{
			name: "kitty ctrl+y followed by raw digit in one chunk",
			in:   []byte("\x1b[121;5u1"),
			want: []byte{ctrlY, '1'},
		},
		{
			name: "kitty ctrl+y with shifted alternate codepoint",
			in:   []byte("\x1b[121:89;5u"),
			want: []byte{ctrlY},
		},
		{
			name: "kitty ctrl+shift+y still maps to prefix (shift ignored, as in legacy)",
			in:   []byte("\x1b[121;6u"),
			want: []byte{ctrlY},
		},
		{
			name: "kitty ctrl+p is not the prefix and passes through",
			in:   []byte("\x1b[112;5u"),
			want: []byte("\x1b[112;5u"),
		},
		{
			name: "kitty plain y without ctrl passes through",
			in:   []byte("\x1b[121u"),
			want: []byte("\x1b[121u"),
		},
		{
			name: "kitty y with shift only (no ctrl) passes through",
			in:   []byte("\x1b[121;2u"),
			want: []byte("\x1b[121;2u"),
		},
		{
			name: "ordinary text passes through unchanged",
			in:   []byte("hello"),
			want: []byte("hello"),
		},
		{
			name: "arrow-key escape sequence passes through unchanged",
			in:   []byte("\x1b[A"),
			want: []byte("\x1b[A"),
		},
		{
			name: "incomplete kitty sequence (no final byte) passes through unchanged",
			in:   []byte("\x1b[121;5"),
			want: []byte("\x1b[121;5"),
		},
		{
			name: "empty input passes through",
			in:   []byte{},
			want: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeKittyPrefix(tt.in)
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("normalizeKittyPrefix(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
