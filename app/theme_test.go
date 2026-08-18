package main

import (
	"errors"
	"image/color"
	"io"
	"testing"
)

func TestParseThemeMode(t *testing.T) {
	cases := []struct {
		in      string
		want    ThemeMode
		wantErr bool
	}{
		{"", ThemeAuto, false},
		{"auto", ThemeAuto, false},
		{"AUTO", ThemeAuto, false},
		{"dark", ThemeDark, false},
		{"light", ThemeLight, false},
		{" Light ", ThemeLight, false},
		{"solarized", "", true},
	}
	for _, c := range cases {
		got, err := parseThemeMode(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseThemeMode(%q): want error, got %q", c.in, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("parseThemeMode(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

func TestResolveThemeMode(t *testing.T) {
	probeLight := func() (bool, bool) { return false, true }
	probeDark := func() (bool, bool) { return true, true }
	probeNoAnswer := func() (bool, bool) { return false, false }

	cases := []struct {
		name    string
		mode    ThemeMode
		d       themeDetector
		want    ThemeMode
		wantSrc themeSource
	}{
		{"explicit dark beats everything", ThemeDark, themeDetector{colorFGBG: "0;15", probe: probeLight}, ThemeDark, themeSourceExplicit},
		{"explicit light beats everything", ThemeLight, themeDetector{probe: probeDark}, ThemeLight, themeSourceExplicit},
		{"probe wins over COLORFGBG", ThemeAuto, themeDetector{colorFGBG: "15;0", probe: probeLight}, ThemeLight, themeSourceProbe},
		{"probe dark", ThemeAuto, themeDetector{probe: probeDark}, ThemeDark, themeSourceProbe},
		{"no probe answer falls to COLORFGBG light", ThemeAuto, themeDetector{colorFGBG: "0;15", probe: probeNoAnswer}, ThemeLight, themeSourceColorFGBG},
		{"nil probe uses COLORFGBG dark", ThemeAuto, themeDetector{colorFGBG: "15;0"}, ThemeDark, themeSourceColorFGBG},
		{"rxvt three-field form", ThemeAuto, themeDetector{colorFGBG: "0;default;15"}, ThemeLight, themeSourceColorFGBG},
		{"nothing answers falls back dark", ThemeAuto, themeDetector{}, ThemeDark, themeSourceFallback},
		{"garbage COLORFGBG falls back dark", ThemeAuto, themeDetector{colorFGBG: "default"}, ThemeDark, themeSourceFallback},
	}
	for _, c := range cases {
		got, src := resolveThemeMode(c.mode, c.d)
		if got != c.want || src != c.wantSrc {
			t.Errorf("%s: resolveThemeMode = %q, %q; want %q, %q", c.name, got, src, c.want, c.wantSrc)
		}
	}
	if themeSourceFallback.detected() {
		t.Error("fallback must not report as detected")
	}
	if !themeSourceProbe.detected() {
		t.Error("probe result must report as detected")
	}
}

func TestThemeModeFromColorFGBG(t *testing.T) {
	cases := []struct {
		in   string
		want ThemeMode
		ok   bool
	}{
		{"15;0", ThemeDark, true},
		{"0;15", ThemeLight, true},
		{"0;default;15", ThemeLight, true},
		{"15;default;0", ThemeDark, true},
		{"7;8", ThemeDark, true},  // 8 is the dark half
		{"0;7", ThemeLight, true}, // 7 is the light half
		{"", "", false},
		{"default", "", false},
		{"15;#ffffff", "", false},
		{"15;16", "", false}, // out of the 16-color cube
	}
	for _, c := range cases {
		got, ok := themeModeFromColorFGBG(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("themeModeFromColorFGBG(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestIsDarkColor(t *testing.T) {
	cases := []struct {
		name string
		c    color.Color
		want bool
	}{
		{"nil is dark", nil, true},
		{"black", color.RGBA{0, 0, 0, 255}, true},
		{"white", color.RGBA{255, 255, 255, 255}, false},
		{"terminal near-black", color.RGBA{0x0B, 0x0B, 0x12, 255}, true},
		{"cream", color.RGBA{0xFD, 0xF6, 0xE3, 255}, false},
		{"mid-grey counts as dark", color.RGBA{0x7F, 0x7F, 0x7F, 255}, true},
	}
	for _, c := range cases {
		if got := isDarkColor(c.c); got != c.want {
			t.Errorf("%s: isDarkColor = %v, want %v", c.name, got, c.want)
		}
	}
}

// chunkReader yields its chunks one Read at a time, then err — the shape a
// real terminal reply arrives in.
type chunkReader struct {
	chunks [][]byte
	err    error
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		if r.err == nil {
			return 0, io.EOF
		}
		return 0, r.err
	}
	n := copy(p, r.chunks[0])
	if n == len(r.chunks[0]) {
		r.chunks = r.chunks[1:]
	} else {
		r.chunks[0] = r.chunks[0][n:]
	}
	return n, nil
}

func TestReadThemeReply(t *testing.T) {
	rgb := func(r, g, b uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: 0xFF} }

	cases := []struct {
		name   string
		chunks [][]byte
		want   color.Color
		ok     bool
	}{
		{
			"xterm 16-bit reply with ST",
			[][]byte{[]byte("\x1b]11;rgb:0b0b/0b0b/1212\x1b\\\x1b[?1;2c")},
			rgb(0x0B, 0x0B, 0x12), true,
		},
		{
			"8-bit reply with BEL",
			[][]byte{[]byte("\x1b]11;rgb:fd/f6/e3\a")},
			rgb(0xFD, 0xF6, 0xE3), true,
		},
		{
			"hex reply",
			[][]byte{[]byte("\x1b]11;#1A2B3C\x1b\\")},
			rgb(0x1A, 0x2B, 0x3C), true,
		},
		{
			"reply split across reads",
			[][]byte{[]byte("\x1b]11;rgb:ff"), []byte("ff/ffff/"), []byte("ffff\x1b\\")},
			rgb(0xFF, 0xFF, 0xFF), true,
		},
		{
			"DA1 alone means no answer",
			[][]byte{[]byte("\x1b[?65;1;9c")},
			nil, false,
		},
		{
			"EOF with nothing",
			nil,
			nil, false,
		},
	}
	for _, c := range cases {
		got, ok := readThemeReply(&chunkReader{chunks: c.chunks})
		if ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
			continue
		}
		if ok && got != c.want {
			t.Errorf("%s: color = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestReadThemeReplyCancelled(t *testing.T) {
	if _, ok := readThemeReply(&chunkReader{err: errors.New("read canceled")}); ok {
		t.Error("a cancelled read must report no answer")
	}
}

func TestScaleHexChannel(t *testing.T) {
	cases := []struct {
		in   string
		want uint8
		ok   bool
	}{
		{"0b", 0x0B, true},
		{"0b0b", 0x0B, true}, // 16-bit form scales down, not truncates
		{"f", 0xFF, true},    // 4-bit form scales up
		{"ffff", 0xFF, true},
		{"", 0, false},
		{"12345", 0, false},
		{"zz", 0, false},
	}
	for _, c := range cases {
		got, ok := scaleHexChannel(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("scaleHexChannel(%q) = 0x%02X, %v; want 0x%02X, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
