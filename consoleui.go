package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const maxLogLines = 500

var (
	termOut io.Writer = os.Stderr
	termUI  *consoleUI
)

type consoleUI struct {
	mu sync.Mutex

	in      *os.File
	out     io.Writer
	enabled bool
	raw     bool
	saved   any

	rows, cols int
	logLines   []string

	buf       []rune
	cursor    int
	history   []string
	histIdx   int
	histDraft []rune

	lastCR bool

	submit chan string
	quit   chan struct{}
	cancel context.CancelFunc

	readOnce sync.Once

	meter func() (level, peak float32)
}

func newConsoleUI() *consoleUI {
	if !ttyIsTerminal(int(os.Stdin.Fd())) || !ttyIsTerminal(int(os.Stderr.Fd())) {
		return nil
	}
	return &consoleUI{
		in:     os.Stdin,
		out:    os.Stderr,
		submit: make(chan string, 16),
		quit:   make(chan struct{}),
	}
}

func (ui *consoleUI) Enable(ctx context.Context, cancel context.CancelFunc) error {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if ui.enabled {
		return nil
	}
	ui.cancel = cancel
	rows, cols := ttySize(int(ui.in.Fd()))
	if rows < 6 || cols < 12 {
		return fmt.Errorf("terminal too small (%dx%d) for the console UI", rows, cols)
	}
	st, err := ttyMakeRaw(int(ui.in.Fd()))
	if err != nil {
		return err
	}
	ui.saved = st
	ui.raw = true
	ui.rows, ui.cols = rows, cols
	ui.enabled = true
	fmt.Fprint(ui.out, "\033[?1049h\033[2J\033[H\033[?25l")
	ui.applyScrollRegionLocked()
	ui.drawLogRegionLocked()
	ui.drawBoxLocked()
	ui.readOnce.Do(func() { go ui.readLoop() })
	go ui.meterLoop()
	return nil
}

func (ui *consoleUI) Disable() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if !ui.enabled {
		return
	}
	ui.enabled = false
	if ui.raw {
		ttyRestore(int(ui.in.Fd()), ui.saved)
		ui.raw = false
	}
	fmt.Fprint(ui.out, "\033[r\033[2J\033[H\033[?25h\033[?1049l")
	ui.signalQuitLocked()
}

func (ui *consoleUI) ReadLine(ctx context.Context) (string, bool) {
	select {
	case <-ctx.Done():
		return "", false
	case <-ui.quit:
		return "", false
	case line := <-ui.submit:
		return line, true
	}
}

func (ui *consoleUI) Write(p []byte) (int, error) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if !ui.enabled {
		return ui.out.Write(p)
	}
	s := strings.TrimSuffix(string(p), "\n")
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	if ui.refreshSizeLocked() {
		for _, line := range lines {
			ui.appendLogLocked(line)
		}
		ui.redrawLocked()
		return len(p), nil
	}
	for _, line := range lines {
		ui.appendLogLocked(line)
		fmt.Fprintf(ui.out, "\033[%d;1H\033[K%s\n", ui.rows-3, truncateToWidth(line, ui.cols-1))
	}
	ui.drawBoxLocked()
	return len(p), nil
}

func (ui *consoleUI) resize() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if !ui.enabled {
		return
	}
	if ui.refreshSizeLocked() {
		ui.redrawLocked()
	}
}

func (ui *consoleUI) refreshSizeLocked() bool {
	rows, cols := ttySize(int(ui.in.Fd()))
	if rows < 6 {
		rows = 6
	}
	if cols < 12 {
		cols = 12
	}
	if rows == ui.rows && cols == ui.cols {
		return false
	}
	ui.rows, ui.cols = rows, cols
	return true
}

func (ui *consoleUI) applyScrollRegionLocked() {
	r := ui.rows - 3
	if r < 1 {
		r = 1
	}
	fmt.Fprintf(ui.out, "\033[1;%dr", r)
}

func (ui *consoleUI) redrawLocked() {
	fmt.Fprint(ui.out, "\033[r\033[2J\033[H")
	ui.applyScrollRegionLocked()
	ui.drawLogRegionLocked()
	ui.drawBoxLocked()
}

func (ui *consoleUI) drawLogRegionLocked() {
	n := ui.rows - 3
	if n < 1 {
		n = 1
	}
	start := len(ui.logLines) - n
	if start < 0 {
		start = 0
	}
	for i := start; i < len(ui.logLines); i++ {
		fmt.Fprintf(ui.out, "\033[%d;1H\033[K%s", 1+i-start, truncateToWidth(ui.logLines[i], ui.cols-1))
	}
}

func (ui *consoleUI) appendLogLocked(line string) {
	ui.logLines = append(ui.logLines, line)
	if len(ui.logLines) > maxLogLines {
		ui.logLines = ui.logLines[len(ui.logLines)-maxLogLines:]
	}
}

func (ui *consoleUI) drawBoxLocked() {
	w := ui.cols - 3
	if w < 1 {
		w = 1
	}
	top := "╭ " + strings.Repeat("─", w)
	bot := "╰ " + strings.Repeat("─", w)
	fmt.Fprint(ui.out, "\033[?25l")
	fmt.Fprintf(ui.out, "\033[%d;1H\033[K\033[90m%s\033[0m", ui.rows-2, top)
	ui.drawMeterLocked()
	fmt.Fprintf(ui.out, "\033[%d;1H\033[K\033[36m#> \033[0m%s", ui.rows-1, string(ui.buf))
	fmt.Fprintf(ui.out, "\033[%d;1H\033[K\033[90m%s\033[0m", ui.rows, bot)
	ui.positionCursorLocked()
	fmt.Fprint(ui.out, "\033[?25h")
}

func (ui *consoleUI) setMeter(level, peak func() float32) {
	ui.mu.Lock()
	if level == nil || peak == nil {
		ui.meter = nil
	} else {
		ui.meter = func() (float32, float32) { return level(), peak() }
	}
	enabled := ui.enabled
	ui.mu.Unlock()
	if enabled {
		ui.mu.Lock()
		ui.drawBoxLocked()
		ui.mu.Unlock()
	}
}

func (ui *consoleUI) meterLoop() {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ui.quit:
			return
		case <-t.C:
			ui.mu.Lock()
			if ui.enabled && ui.meter != nil {
				ui.drawBoxLocked()
			}
			ui.mu.Unlock()
		}
	}
}

func (ui *consoleUI) drawMeterLocked() {
	if ui.meter == nil {
		return
	}
	lvl, peak := ui.meter()
	const segs = 12
	bar := make([]rune, segs)
	for i := range bar {
		if lvl >= float32(i+1)/segs {
			bar[i] = '█'
		} else {
			bar[i] = '░'
		}
	}
	if pi := int(peak * segs); pi >= 0 && pi < segs && bar[pi] == '░' {
		bar[pi] = '▏'
	}
	txt := " 🔊 " + string(bar)
	startCol := ui.cols - runeWidths([]rune(txt)) + 1
	if startCol < 3 {
		return
	}
	fmt.Fprintf(ui.out, "\033[%d;%dH\033[90m%s\033[0m", ui.rows-2, startCol, txt)
}

func (ui *consoleUI) drawInputRowLocked() {
	display := truncateToWidth(string(ui.buf), ui.cols-4)
	fmt.Fprintf(ui.out, "\033[%d;1H\033[K\033[36m#> \033[0m%s", ui.rows-1, display)
	ui.positionCursorLocked()
}

func (ui *consoleUI) positionCursorLocked() {
	displayW := runeWidths([]rune(truncateToWidth(string(ui.buf), ui.cols-4)))
	col := 4 + runeWidths(ui.buf[:ui.cursor])
	if col > 4+displayW {
		col = 4 + displayW
	}
	if col > ui.cols {
		col = ui.cols
	}
	fmt.Fprintf(ui.out, "\033[%d;%dH", ui.rows-1, col)
}

func (ui *consoleUI) readLoop() {
	br := bufio.NewReader(ui.in)
	var seq [8]byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			ui.signalQuit()
			return
		}
		if !ui.dispatchKey(b, br, &seq) {
			return
		}
		ui.mu.Lock()
		if ui.enabled {
			if ui.refreshSizeLocked() {
				ui.redrawLocked()
			} else {
				ui.drawInputRowLocked()
			}
		}
		ui.mu.Unlock()
	}
}

func (ui *consoleUI) dispatchKey(b byte, br *bufio.Reader, seq *[8]byte) bool {
	switch {
	case b == 0x1b:
		return ui.escapeKey(br)
	case b < 0x20 || b == 0x7f:
		return ui.controlKey(b, br)
	case b < utf8.RuneSelf:
		ui.insert(rune(b))
		return true
	default:
		size := utf8SeqLen(b)
		seq[0] = b
		n := 1
		for n < size {
			nb, err := br.ReadByte()
			if err != nil {
				break
			}
			seq[n] = nb
			n++
		}
		if r, _ := utf8.DecodeRune(seq[:n]); r != utf8.RuneError {
			ui.insert(r)
		}
		return true
	}
}

func (ui *consoleUI) controlKey(b byte, br *bufio.Reader) bool {
	switch b {
	case '\r':
		ui.submitLine()
	case '\n':
		ui.mu.Lock()
		if ui.lastCR {
			ui.lastCR = false
			ui.mu.Unlock()
			return true
		}
		ui.mu.Unlock()
		ui.submitLine()
	case 0x7f, 0x08:
		ui.mu.Lock()
		ui.backspaceLocked()
		ui.mu.Unlock()
	case 0x03:
		ui.mu.Lock()
		if ui.cancel != nil {
			ui.cancel()
		}
		ui.mu.Unlock()
		ui.signalQuit()
		return false
	case 0x04:
		ui.signalQuit()
		return false
	case 0x15:
		ui.mu.Lock()
		ui.buf = nil
		ui.cursor = 0
		ui.mu.Unlock()
	case 0x0b:
		ui.mu.Lock()
		ui.buf = ui.buf[:ui.cursor]
		ui.mu.Unlock()
	case 0x01:
		ui.mu.Lock()
		ui.cursor = 0
		ui.mu.Unlock()
	case 0x05:
		ui.mu.Lock()
		ui.cursor = len(ui.buf)
		ui.mu.Unlock()
	}
	return true
}

func (ui *consoleUI) escapeKey(br *bufio.Reader) bool {
	b, err := br.ReadByte()
	if err != nil {
		return true
	}
	switch b {
	case '[':
		var params []byte
		for {
			c, err := br.ReadByte()
			if err != nil {
				return true
			}
			if c >= 0x40 && c <= 0x7e {
				ui.applyCSI(string(params), c)
				return true
			}
			params = append(params, c)
		}
	case 'O':
		c, err := br.ReadByte()
		if err != nil {
			return true
		}
		if c == 'A' || c == 'B' || c == 'C' || c == 'D' || c == 'H' || c == 'F' {
			ui.applyCSI("", c)
		}
		return true
	}
	return true
}

func (ui *consoleUI) applyCSI(params string, final byte) {
	switch final {
	case 'A':
		ui.mu.Lock()
		ui.historyPrevLocked()
		ui.mu.Unlock()
	case 'B':
		ui.mu.Lock()
		ui.historyNextLocked()
		ui.mu.Unlock()
	case 'C':
		ui.mu.Lock()
		if ui.cursor < len(ui.buf) {
			ui.cursor++
		}
		ui.mu.Unlock()
	case 'D':
		ui.mu.Lock()
		if ui.cursor > 0 {
			ui.cursor--
		}
		ui.mu.Unlock()
	case 'H':
		ui.mu.Lock()
		ui.cursor = 0
		ui.mu.Unlock()
	case 'F':
		ui.mu.Lock()
		ui.cursor = len(ui.buf)
		ui.mu.Unlock()
	case '~':
		switch params {
		case "1", "7":
			ui.mu.Lock()
			ui.cursor = 0
			ui.mu.Unlock()
		case "3":
			ui.mu.Lock()
			ui.deleteLocked()
			ui.mu.Unlock()
		case "4", "8":
			ui.mu.Lock()
			ui.cursor = len(ui.buf)
			ui.mu.Unlock()
		}
	}
}

func (ui *consoleUI) insert(r rune) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	ui.lastCR = false
	ui.buf = append(ui.buf, 0)
	copy(ui.buf[ui.cursor+1:], ui.buf[ui.cursor:])
	ui.buf[ui.cursor] = r
	ui.cursor++
}

func (ui *consoleUI) backspaceLocked() {
	if ui.cursor > 0 {
		ui.buf = append(ui.buf[:ui.cursor-1], ui.buf[ui.cursor:]...)
		ui.cursor--
	}
}

func (ui *consoleUI) deleteLocked() {
	if ui.cursor < len(ui.buf) {
		ui.buf = append(ui.buf[:ui.cursor], ui.buf[ui.cursor+1:]...)
	}
}

func (ui *consoleUI) historyPrevLocked() {
	if len(ui.history) == 0 {
		return
	}
	if ui.histIdx == len(ui.history) {
		ui.histDraft = append([]rune(nil), ui.buf...)
	}
	if ui.histIdx > 0 {
		ui.histIdx--
		ui.buf = []rune(ui.history[ui.histIdx])
		ui.cursor = len(ui.buf)
	}
}

func (ui *consoleUI) historyNextLocked() {
	if ui.histIdx >= len(ui.history) {
		return
	}
	ui.histIdx++
	if ui.histIdx == len(ui.history) {
		ui.buf = append([]rune(nil), ui.histDraft...)
	} else {
		ui.buf = []rune(ui.history[ui.histIdx])
	}
	ui.cursor = len(ui.buf)
}

func (ui *consoleUI) submitLine() {
	ui.mu.Lock()
	ui.lastCR = true
	line := string(ui.buf)
	ui.buf = nil
	ui.cursor = 0
	ui.histIdx = len(ui.history)
	if strings.TrimSpace(line) != "" && (len(ui.history) == 0 || ui.history[len(ui.history)-1] != line) {
		ui.history = append(ui.history, line)
	}
	if ui.enabled {
		ui.drawInputRowLocked()
	}
	ui.mu.Unlock()
	select {
	case ui.submit <- line:
	case <-ui.quit:
	}
}

func (ui *consoleUI) signalQuit() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	ui.signalQuitLocked()
}

func (ui *consoleUI) signalQuitLocked() {
	select {
	case <-ui.quit:
	default:
		close(ui.quit)
	}
}

func utf8SeqLen(b byte) int {
	switch {
	case b&0xE0 == 0xC0:
		return 2
	case b&0xF0 == 0xE0:
		return 3
	case b&0xF8 == 0xF0:
		return 4
	}
	return 1
}

func runeWidth(r rune) int {
	if r < 0x20 || r == 0x7f {
		return 0
	}
	switch {
	case r >= 0x1100 && r <= 0x115F,
		r >= 0x2E80 && r <= 0x303E,
		r >= 0x3041 && r <= 0x33FF,
		r >= 0x3400 && r <= 0x4DBF,
		r >= 0x4E00 && r <= 0x9FFF,
		r >= 0xA000 && r <= 0xA4CF,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE4F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1FAFF,
		r >= 0x20000 && r <= 0x3FFFD:
		return 2
	}
	return 1
}

func runeWidths(rs []rune) int {
	w := 0
	for _, r := range rs {
		w += runeWidth(r)
	}
	return w
}

func truncateToWidth(s string, maxW int) string {
	if maxW < 1 {
		return ""
	}
	runes := []rune(s)
	w := 0
	for _, r := range runes {
		w += runeWidth(r)
	}
	if w <= maxW {
		return s
	}
	budget := maxW - 1
	if budget < 0 {
		budget = 0
	}
	w = 0
	end := 0
	for i, r := range runes {
		if w+runeWidth(r) > budget {
			break
		}
		w += runeWidth(r)
		end = i + 1
	}
	return string(runes[:end]) + "…"
}
