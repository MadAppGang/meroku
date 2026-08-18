package main

// theme_probe.go asks the terminal for its background color with an OSC 11
// query, so theme.go can decide between the light and dark palette from the
// live background rather than a stale environment hint.

import (
	"bytes"
	"image/color"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/muesli/cancelreader"
)

// themeProbeTimeout is the whole budget for asking the terminal what color it
// is. A terminal that implements OSC 11 replies in single-digit milliseconds —
// the timeout is never reached on success, only on terminals that ignore the
// query — so the only thing a longer budget buys is a longer stall for the
// people it already fails.
const themeProbeTimeout = 150 * time.Millisecond

// themeBackgroundQuery is the OSC 11 request, followed by a DA1 (primary
// device attributes) request as a SENTINEL.
//
// The sentinel is what makes a tight deadline safe. Terminals answer requests
// in order, so a terminal that does not implement OSC 11 still answers DA1 —
// and its DA1 reply is proof that the OSC 11 answer is never coming. We stop
// there, in a few milliseconds, instead of waiting out the clock. The deadline
// is only the backstop for terminals that answer neither.
const themeBackgroundQuery = "\x1b]11;?\x1b\\\x1b[c"

// terminalBackgroundProbe returns a themeDetector probe that asks the terminal
// for its background color, or nil when there is nothing worth asking.
//
// Both files must be terminals: the request is written to out and the reply
// read from in, which is the same device in the normal case. If in were a pipe
// we would be reading the user's piped stdin looking for an escape sequence
// that cannot be there, and consuming it.
func terminalBackgroundProbe(in, out term.File) func() (bool, bool) {
	if in == nil || out == nil || !term.IsTerminal(in.Fd()) || !term.IsTerminal(out.Fd()) {
		return nil
	}
	return func() (bool, bool) {
		bg, ok := queryTerminalBackground(in, out)
		if !ok {
			return false, false
		}
		return isDarkColor(bg), true
	}
}

// queryTerminalBackground runs one request/response round trip. It always
// restores the terminal state it changed — a probe that leaves a terminal in
// raw mode is far worse than a probe that returns no answer.
//
// The read is bounded with a cancel reader, not os.File.SetReadDeadline:
// Go does not register stdin/stdout/stderr with its runtime poller, so on a
// real terminal SetReadDeadline returns "file type does not support deadline"
// — on every terminal, not just the awkward ones.
func queryTerminalBackground(in, out term.File) (color.Color, bool) {
	rd, err := cancelreader.NewReader(in)
	if err != nil {
		return nil, false
	}
	defer rd.Close() //nolint:errcheck

	state, err := term.MakeRaw(in.Fd())
	if err != nil {
		return nil, false
	}
	defer term.Restore(in.Fd(), state) //nolint:errcheck

	if _, err := out.Write([]byte(themeBackgroundQuery)); err != nil {
		return nil, false
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
		case <-time.After(themeProbeTimeout):
			rd.Cancel()
		}
	}()

	return readThemeReply(rd)
}

// readThemeReply accumulates until it has an OSC 11 color, sees the DA1
// sentinel, or the reader stops. Split out from queryTerminalBackground so it
// can be tested without a pty — including the case a terminal actually
// produces, where the reply arrives split across several reads.
func readThemeReply(r io.Reader) (color.Color, bool) {
	var acc []byte
	var buf [256]byte
	for {
		n, err := r.Read(buf[:])
		if n > 0 {
			acc = append(acc, buf[:n]...)
			if c, ok := parseOSC11(acc); ok {
				return c, true
			}
			// The sentinel came back without an OSC 11 reply: this terminal
			// has told us, as clearly as it can, that it will not answer.
			if hasDA1Reply(acc) {
				return nil, false
			}
		}
		if err != nil {
			return nil, false // cancelled, EOF, or a read error — all "no answer"
		}
		if len(acc) > 4096 {
			return nil, false // runaway input; not our reply
		}
	}
}

// parseOSC11 finds an OSC 11 background-color reply in buf.
//
// The payload is a color spec after "]11;": either the X11 "rgb:" form with
// 1-4 hex digits per channel (xterm answers "rgb:0b0b/0b0b/1212", others
// "rgb:0b/0b/12") or a plain "#RRGGBB". Anything else is not an answer.
func parseOSC11(buf []byte) (color.Color, bool) {
	i := bytes.Index(buf, []byte("]11;"))
	if i < 0 {
		return nil, false
	}
	rest := buf[i+len("]11;"):]
	// The reply ends at ST (ESC \) or BEL; without a terminator it is still
	// arriving, so report "not yet" and let the caller read more.
	end := bytes.IndexAny(rest, "\x1b\a")
	if end < 0 {
		return nil, false
	}
	return parseOSC11ColorSpec(strings.TrimSpace(string(rest[:end])))
}

func parseOSC11ColorSpec(spec string) (color.Color, bool) {
	if after, ok := strings.CutPrefix(spec, "rgb:"); ok {
		parts := strings.Split(after, "/")
		if len(parts) != 3 {
			return nil, false
		}
		var ch [3]uint8
		for i, p := range parts {
			v, ok := scaleHexChannel(p)
			if !ok {
				return nil, false
			}
			ch[i] = v
		}
		return color.RGBA{R: ch[0], G: ch[1], B: ch[2], A: 0xFF}, true
	}
	if after, ok := strings.CutPrefix(spec, "#"); ok && len(after) == 6 {
		v, err := strconv.ParseUint(after, 16, 32)
		if err != nil {
			return nil, false
		}
		return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xFF}, true
	}
	return nil, false
}

// scaleHexChannel reads one X11 channel of 1-4 hex digits and scales it to 8
// bits. "0b0b" and "0b" must both mean 0x0B: the digits are a FRACTION of the
// channel's full width, not a fixed-width integer, so the width decides the
// divisor.
func scaleHexChannel(s string) (uint8, bool) {
	if len(s) == 0 || len(s) > 4 {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, false
	}
	maxV := uint64(1)<<(4*len(s)) - 1
	return uint8(v * 0xFF / maxV), true
}

// hasDA1Reply reports whether buf contains a primary-device-attributes reply,
// "ESC [ ? ... c" — our sentinel that the terminal has finished answering.
func hasDA1Reply(buf []byte) bool {
	i := bytes.Index(buf, []byte("\x1b[?"))
	if i < 0 {
		return false
	}
	return bytes.IndexByte(buf[i:], 'c') >= 0
}
