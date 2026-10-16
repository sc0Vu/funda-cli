package gameimage

import (
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// Render writes inline image bytes for explicitly selected protocols.
// Auto detection should resolve to a concrete mode before calling Render.
// Images are limited to 2 MiB to avoid flooding terminal escape buffers.
func Render(w io.Writer, mode Mode, data []byte, photoURL string) error {
	if mode == None {
		return nil
	}
	if mode == Link || mode == Auto {
		return renderLink(w, photoURL)
	}
	if len(data) == 0 {
		return renderLink(w, photoURL)
	}
	if len(data) > 2<<20 {
		return fmt.Errorf("image exceeds 2 MiB terminal limit")
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	switch mode {
	case Kitty:
		// Kitty requires base64 chunks <= 4096 bytes, with m=1 for continuation.
		for len(encoded) > 0 {
			n := len(encoded)
			if n > 4096 {
				n = 4096
			}
			more := 0
			if n < len(encoded) {
				more = 1
			}
			if _, err := fmt.Fprintf(w, "\x1b_Ga=T,f=100,t=d,m=%d;%s\x1b\\", more, encoded[:n]); err != nil {
				return err
			}
			encoded = encoded[n:]
		}
		return nil
	case ITerm2:
		_, err := fmt.Fprintf(w, "\x1b]1337;File=inline=1;size=%d:%s\a\n", len(data), encoded)
		return err
	default:
		return fmt.Errorf("unsupported image mode %q", mode)
	}
}
func renderLink(w io.Writer, raw string) error {
	if err := SafePhotoURL(raw); err != nil {
		_, writeErr := io.WriteString(w, "Photo unavailable; open the original listing in your browser.\n")
		return writeErr
	}
	_, err := io.WriteString(w, "Photo: "+strings.TrimSpace(raw)+"\n")
	return err
}
