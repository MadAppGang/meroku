package main

// theme.go answers one question — is meroku drawing onto a dark terminal or a
// light one — and publishes the answer to lipgloss before anything renders.
//
// The decision is made once, in applyTheme (called from main right after flag
// parsing), by asking the most trustworthy source first:
//
//	--theme flag / MEROKU_THEME  →  OSC 11 query  →  COLORFGBG  →  dark
//
// The OSC 11 query (theme_probe.go) asks the terminal what its background is
// RIGHT NOW. COLORFGBG is a hint the terminal or a shell profile exported at
// some point, and it goes stale when the user changes theme — it is consulted
// only when the terminal did not answer. Dark is the last resort because it is
// the majority default among terminals AND the palette meroku was designed in:
// guessing it wrong costs contrast, while guessing light wrong on a dark
// terminal costs legibility outright.
//
// The answer lands in lipgloss.SetHasDarkBackground, which pre-empts lipgloss's
// own lazy background query (slow, and unsafe once a Bubble Tea program owns
// stdin) and makes every lipgloss.AdaptiveColor in the app resolve correctly —
// including the ones inside huh's built-in form themes.

import (
	"fmt"
	"image/color"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

// ThemeMode is the background meroku should draw for.
type ThemeMode string

const (
	// ThemeAuto defers to resolveThemeMode, which picks dark or light from the
	// terminal. It is never an answer — every consumer sees dark or light.
	ThemeAuto ThemeMode = "auto"
	// ThemeDark is the original design: bright hues on a near-black page.
	ThemeDark ThemeMode = "dark"
	// ThemeLight is the same layout drawn for a white page: deeper hues, dark
	// text, pale panels.
	ThemeLight ThemeMode = "light"
)

// parseThemeMode reads a --theme / MEROKU_THEME value. The empty string is
// auto, so an unset knob and an explicit "auto" agree.
func parseThemeMode(s string) (ThemeMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(ThemeAuto):
		return ThemeAuto, nil
	case string(ThemeDark):
		return ThemeDark, nil
	case string(ThemeLight):
		return ThemeLight, nil
	}
	return "", fmt.Errorf("invalid theme %q: want auto, light or dark", s)
}

// themeSource says where a resolved mode came from. It exists so the caller
// can tell a detected answer from a guess: falling back to dark on a light
// terminal is the one failure this file can produce, and it is invisible —
// nothing errors, the colors are just wrong.
type themeSource string

const (
	themeSourceExplicit  themeSource = "explicit"
	themeSourceProbe     themeSource = "query"
	themeSourceColorFGBG themeSource = "COLORFGBG"
	themeSourceFallback  themeSource = "fallback"
)

// detected reports whether the mode came from an actual reading rather than a
// default.
func (s themeSource) detected() bool { return s != themeSourceFallback }

// themeDetector carries what resolveThemeMode may not fetch for itself, so the
// resolution logic stays testable without a terminal or environment.
type themeDetector struct {
	// colorFGBG is the COLORFGBG environment value, or "" when unset.
	colorFGBG string
	// probe asks the terminal itself and reports (dark, ok); ok is false when
	// the terminal did not answer. A nil probe skips the query entirely — the
	// correct choice whenever output is not an interactive terminal, where the
	// query could only ever time out.
	probe func() (dark bool, ok bool)
}

// resolveThemeMode collapses ThemeAuto to a concrete mode. An explicit dark or
// light is returned untouched — the whole point of the flag is that it beats
// detection.
func resolveThemeMode(m ThemeMode, d themeDetector) (ThemeMode, themeSource) {
	if m != ThemeAuto {
		return m, themeSourceExplicit
	}
	if d.probe != nil {
		if dark, ok := d.probe(); ok {
			return themeModeFor(dark), themeSourceProbe
		}
	}
	if mode, ok := themeModeFromColorFGBG(d.colorFGBG); ok {
		return mode, themeSourceColorFGBG
	}
	return ThemeDark, themeSourceFallback
}

func themeModeFor(dark bool) ThemeMode {
	if dark {
		return ThemeDark
	}
	return ThemeLight
}

// themeModeFromColorFGBG reads the de-facto COLORFGBG convention:
// semicolon-joined fields whose LAST field is the background as an ANSI color
// index — "15;0" (light text on black) from most terminals, "0;default;15"
// (dark text on white) from the rxvt family that wedges a decoration slot in
// the middle.
//
// Indices 0-6 and 8 are the dark half of the 16-color cube, 7 and 9-15 the
// light half. Anything else — "default", a truecolor spelling, an empty value —
// is not an answer, and reports ok=false so resolution falls through rather
// than inventing one.
func themeModeFromColorFGBG(v string) (ThemeMode, bool) {
	fields := strings.Split(strings.TrimSpace(v), ";")
	last := strings.TrimSpace(fields[len(fields)-1])
	if last == "" {
		return "", false
	}
	n, err := strconv.Atoi(last)
	if err != nil || n < 0 || n > 15 {
		return "", false
	}
	return themeModeFor(n <= 6 || n == 8), true
}

// isDarkColor reports whether c is a dark color, by HSL lightness — the same
// measure lipgloss uses internally. Half-lit is called dark: a mid-grey
// terminal is closer to the dark design than to the light one.
func isDarkColor(c color.Color) bool {
	if c == nil {
		return true
	}
	r, g, b, _ := c.RGBA()
	// RGBA returns 16-bit alpha-premultiplied channels; normalize to 0..1.
	rf, gf, bf := float64(r)/0xFFFF, float64(g)/0xFFFF, float64(b)/0xFFFF
	hi, lo := rf, rf
	for _, v := range [2]float64{gf, bf} {
		if v > hi {
			hi = v
		}
		if v < lo {
			lo = v
		}
	}
	return (hi+lo)/2 < 0.5
}

// applyTheme resolves the theme once per invocation and hands the answer to
// lipgloss. It must run before anything renders and before any Bubble Tea
// program takes over stdin — the probe does a raw-mode read on stdin that
// would otherwise race the TUI's own input loop.
func applyTheme() {
	mode, err := parseThemeMode(themeValue())
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  %v, using auto\n", err)
		mode = ThemeAuto
	}
	d := newThemeDetector()
	resolved, src := resolveThemeMode(mode, d)
	lipgloss.SetHasDarkBackground(resolved == ThemeDark)
	if os.Getenv("MEROKU_THEME_DEBUG") != "" {
		// Undocumented on purpose: a support switch for "auto picked the wrong
		// one", which is otherwise invisible — the whole failure mode is that
		// nothing errors and the colors are simply wrong.
		fmt.Fprintf(os.Stderr, "theme-debug: requested=%s resolved=%s source=%s colorfgbg=%q probe=%v stdin_tty=%v stdout_tty=%v\n",
			mode, resolved, src, d.colorFGBG, d.probe != nil,
			term.IsTerminal(os.Stdin.Fd()), term.IsTerminal(os.Stdout.Fd()))
	}
	if mode == ThemeAuto && !src.detected() && term.IsTerminal(os.Stdout.Fd()) {
		// The one silent failure: a light terminal gets the dark palette with
		// no hint that a guess was made. The fix it names also silences it.
		fmt.Fprintln(os.Stderr, "theme: your terminal did not report its background color, using dark — set MEROKU_THEME=light or pass -theme light if that is wrong")
	}
}

// themeValue is the flag, or MEROKU_THEME when the flag is untouched.
func themeValue() string {
	if *themeFlag != "" && *themeFlag != string(ThemeAuto) {
		return *themeFlag
	}
	if v := os.Getenv("MEROKU_THEME"); v != "" {
		return v
	}
	return *themeFlag
}

// newThemeDetector assembles what resolveThemeMode may not fetch for itself,
// and decides whether the terminal is worth querying. The TUI renders on
// stdout, so that is the stream we ask about, with stdin as the reply channel —
// the same device in the normal case, and both must be terminals or we would
// be reading someone's pipe.
func newThemeDetector() themeDetector {
	d := themeDetector{colorFGBG: os.Getenv("COLORFGBG")}
	if os.Getenv("NO_COLOR") != "" {
		return d
	}
	if t := os.Getenv("TERM"); t == "" || t == "dumb" {
		return d
	}
	d.probe = terminalBackgroundProbe(os.Stdin, os.Stdout)
	return d
}

// adapt is shorthand for a color that resolves per terminal background: light
// terminals get the first value, dark terminals the second. Values are ANSI
// 256 indices or hex strings, same as lipgloss.Color.
func adapt(light, dark string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Light: light, Dark: dark}
}

// theme is the app-wide palette. Dark values are the original meroku design;
// light values keep the hue and drop the lightness so the same screen reads on
// a white page.
//
// Deliberately NOT here: badge ink. White-on-red or black-on-yellow chips keep
// their literal lipgloss.Color foreground — the badge background provides the
// contrast, not the page, so adapting the ink would break them.
var theme = struct {
	// Status
	Error      lipgloss.AdaptiveColor // bright red accents, errors, deletions
	ErrorDeep  lipgloss.AdaptiveColor // ANSI "9"-style red
	Success    lipgloss.AdaptiveColor // bright green, checkmarks, creations
	SuccessAlt lipgloss.AdaptiveColor // secondary green
	Warning    lipgloss.AdaptiveColor // orange, cautions, updates
	Yellow     lipgloss.AdaptiveColor // bright yellow highlights
	Gold       lipgloss.AdaptiveColor // gold titles and keys

	// Accents
	Info   lipgloss.AdaptiveColor // primary blue
	Blue   lipgloss.AdaptiveColor // deeper blue
	Cyan   lipgloss.AdaptiveColor // bright cyan headings
	Teal   lipgloss.AdaptiveColor // spring-green/teal accents
	Purple lipgloss.AdaptiveColor // purple accents
	Pink   lipgloss.AdaptiveColor // pink/magenta accents

	// Text, brightest to faintest
	TextStrong lipgloss.AdaptiveColor // emphasized foreground (was #ffffff)
	Text       lipgloss.AdaptiveColor // primary foreground (was 252)
	Muted      lipgloss.AdaptiveColor // secondary text (was 245)
	Dim        lipgloss.AdaptiveColor // tertiary text (was 243)
	Faint      lipgloss.AdaptiveColor // hints, disabled (was 241)
	Border     lipgloss.AdaptiveColor // rules, dividers (was 240)

	// Surfaces
	Panel    lipgloss.AdaptiveColor // subtle panel background (was 235)
	PanelAlt lipgloss.AdaptiveColor // slightly raised background (was 237)
}{
	Error:      adapt("160", "196"),
	ErrorDeep:  adapt("124", "9"),
	Success:    adapt("28", "82"),
	SuccessAlt: adapt("29", "42"),
	Warning:    adapt("166", "214"),
	Yellow:     adapt("136", "226"),
	Gold:       adapt("136", "220"),

	Info:   adapt("26", "39"),
	Blue:   adapt("25", "33"),
	Cyan:   adapt("31", "87"),
	Teal:   adapt("30", "86"),
	Purple: adapt("55", "99"),
	Pink:   adapt("161", "205"),

	TextStrong: adapt("#000000", "#ffffff"),
	Text:       adapt("235", "252"),
	Muted:      adapt("240", "245"),
	Dim:        adapt("241", "243"),
	Faint:      adapt("246", "241"),
	Border:     adapt("249", "240"),

	Panel:    adapt("254", "235"),
	PanelAlt: adapt("252", "237"),
}
