package gameimage

import "testing"

func TestModes(t *testing.T) {
	for _, s := range []string{"auto", "kitty", "iterm2", "link", "none"} {
		if _, e := Parse(s); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := Parse("broken"); e == nil {
		t.Fatal("accepted invalid mode")
	}
}
func TestDetection(t *testing.T) {
	cases := []struct {
		env  map[string]string
		tty  bool
		want Mode
	}{
		{map[string]string{"KITTY_WINDOW_ID": "123"}, true, Kitty},
		{map[string]string{"TERM_PROGRAM": "iTerm.app"}, true, ITerm2},
		{map[string]string{"TERM_PROGRAM": "iTerm.app", "TMUX": "1"}, true, Link},
		{map[string]string{"KITTY_WINDOW_ID": "1"}, false, Link},
		{map[string]string{"TERM": "dumb"}, true, Link},
		{map[string]string{}, true, Link},
	}
	for _, c := range cases {
		if got := Detect(c.env, c.tty); got != c.want {
			t.Errorf("got %s want %s", got, c.want)
		}
	}
}
func TestSafePhotoURL(t *testing.T) {
	for _, s := range []string{"file:///etc/passwd", "javascript:alert(1)", "https://user:pass@example.com/img", "https://example.com/\x1b"} {
		if SafePhotoURL(s) == nil {
			t.Errorf("accepted %q", s)
		}
	}
	if err := SafePhotoURL("https://example.com/photo.jpg"); err != nil {
		t.Fatal(err)
	}
}
