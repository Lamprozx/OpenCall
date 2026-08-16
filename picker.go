package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

type pickerTerm struct {
	in  *bufio.Reader
	out io.Writer

	rows, cols int
	startRow   int
	restore    func()

	block int
}

func openPicker() (*pickerTerm, bool) {
	if !ttyIsTerminal(int(os.Stdin.Fd())) || !ttyIsTerminal(int(os.Stderr.Fd())) {
		return nil, false
	}
	st, err := ttyMakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, false
	}
	rows, cols := ttySize(int(os.Stdin.Fd()))
	startRow := queryCursorRow()
	if startRow < 1 || startRow > rows {
		startRow = (rows + 1) / 2
	}
	return &pickerTerm{
		in:       bufio.NewReader(os.Stdin),
		out:      os.Stderr,
		rows:     rows,
		cols:     cols,
		startRow: startRow,
		restore:  func() { ttyRestore(int(os.Stdin.Fd()), st) },
	}, true
}

func (pt *pickerTerm) close() {
	pt.eraseBlock()
	if pt.restore != nil {
		pt.restore()
	}
	fmt.Fprint(pt.out, "\033[?25h")
}

func (pt *pickerTerm) hideCursor() { fmt.Fprint(pt.out, "\033[?25l") }
func (pt *pickerTerm) showCursor() { fmt.Fprint(pt.out, "\033[?25h") }

func queryCursorRow() int {
	fmt.Fprint(os.Stderr, "\033[6n")
	f := os.Stdin
	_ = f.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	defer f.SetReadDeadline(time.Time{})
	var buf []byte
	one := make([]byte, 1)
	for len(buf) < 64 {
		n, err := f.Read(one)
		if err != nil || n == 0 {
			return 0
		}
		buf = append(buf, one[0])
		if one[0] == 'R' {
			break
		}
	}
	s := string(buf)
	i := strings.IndexByte(s, '[')
	j := strings.IndexByte(s, ';')
	if i < 0 || j <= i {
		return 0
	}
	var row int
	if _, err := fmt.Sscanf(s[i+1:j], "%d", &row); err != nil {
		return 0
	}
	return row
}

func (pt *pickerTerm) ensureRoom(needed int) {
	lastRow := pt.startRow + needed - 1
	if lastRow <= pt.rows {
		return
	}
	scroll := lastRow - pt.rows
	if down := pt.rows - pt.startRow; down > 0 {
		fmt.Fprintf(pt.out, "\033[%dB", down)
	}
	for i := 0; i < scroll; i++ {
		fmt.Fprint(pt.out, "\n")
	}
	pt.startRow = pt.rows - needed + 1
	if pt.startRow < 1 {
		pt.startRow = 1
	}
	if up := needed - 1; up > 0 {
		fmt.Fprintf(pt.out, "\033[%dA", up)
	}
}

func (pt *pickerTerm) drawBlock(lines []string) {
	for i, l := range lines {
		if i > 0 {
			fmt.Fprint(pt.out, "\n")
		}
		fmt.Fprint(pt.out, "\r\033[K")
		fmt.Fprint(pt.out, l)
	}
	pt.block = len(lines)
}

func (pt *pickerTerm) redrawBlock(lines []string) {
	old := pt.block
	if old == 0 {
		pt.drawBlock(lines)
		return
	}
	fmt.Fprintf(pt.out, "\033[%dA", old-1)
	n := old
	if len(lines) > n {
		n = len(lines)
	}
	for i := 0; i < n; i++ {
		if i > 0 {
			fmt.Fprint(pt.out, "\n")
		}
		fmt.Fprint(pt.out, "\r\033[K")
		if i < len(lines) {
			fmt.Fprint(pt.out, lines[i])
		}
	}
	if n > len(lines) {
		fmt.Fprintf(pt.out, "\033[%dA", n-len(lines))
	}
	pt.block = len(lines)
}

func (pt *pickerTerm) eraseBlock() {
	if pt.block == 0 {
		return
	}
	fmt.Fprintf(pt.out, "\033[%dA", pt.block-1)
	for i := 0; i < pt.block; i++ {
		if i > 0 {
			fmt.Fprint(pt.out, "\n")
		}
		fmt.Fprint(pt.out, "\r\033[K")
	}
	fmt.Fprintf(pt.out, "\033[%dA", pt.block-1)
	pt.block = 0
}

func (pt *pickerTerm) arrowKey() (dir byte, ok bool) {
	b, err := pt.in.ReadByte()
	if err != nil {
		return 0, false
	}
	if b == '[' || b == 'O' {
		c, err := pt.in.ReadByte()
		if err != nil {
			return 0, false
		}
		switch c {
		case 'A', 'B', 'C', 'D':
			return c, true
		}
	}
	return 0, false
}

func (pt *pickerTerm) pickList(title, header string, items []string, footer string) (idx int, key byte, ok bool) {
	if len(items) == 0 {
		return 0, 'q', false
	}
	idx = 0
	pt.hideCursor()
	maxVis := 5
	if len(items) < maxVis {
		maxVis = len(items)
	}
	pt.ensureRoom(2 + maxVis)
	for {
		pt.drawList(title, header, items, footer, idx)
		b, err := pt.in.ReadByte()
		if err != nil {
			pt.eraseBlock()
			pt.showCursor()
			return idx, 'q', false
		}
		switch {
		case b == 'q' || b == 'Q' || b == 0x03:
			pt.eraseBlock()
			pt.showCursor()
			return idx, 'q', false
		case b == '\r' || b == '\n':
			pt.eraseBlock()
			pt.showCursor()
			return idx, '\r', true
		case b == 'j' || b == 'J':
			if idx < len(items)-1 {
				idx++
			}
		case b == 'k' || b == 'K':
			if idx > 0 {
				idx--
			}
		case b == 0x1b:
			if dir, aok := pt.arrowKey(); aok {
				switch dir {
				case 'A':
					if idx > 0 {
						idx--
					}
				case 'B':
					if idx < len(items)-1 {
						idx++
					}
				}
			} else {
				pt.eraseBlock()
				pt.showCursor()
				return idx, 0x1b, false
			}
		default:
			pt.eraseBlock()
			pt.showCursor()
			return idx, b, true
		}
	}
}

func (pt *pickerTerm) drawList(title, header string, items []string, footer string, idx int) {
	vis := len(items)
	if room := pt.rows - pt.startRow - 1; vis > room {
		vis = room
	}
	if vis < 1 {
		vis = 1
	}
	winStart := 0
	if len(items) > vis {
		winStart = idx - vis/2
		if winStart < 0 {
			winStart = 0
		}
		if winStart+vis > len(items) {
			winStart = len(items) - vis
		}
	}
	titleRow := "\033[1;36m? \033[0m" + truncateToWidth(title, pt.cols-4)
	if footer != "" {
		titleRow += "  \033[90m" + truncateToWidth(footer, pt.cols-2) + "\033[0m"
	}
	lines := []string{titleRow}
	if header != "" {
		lines = append(lines, "\033[90m"+truncateToWidth(header, pt.cols-2)+"\033[0m")
	}
	for i := winStart; i < winStart+vis && i < len(items); i++ {
		if i == idx {
			lines = append(lines, "\033[1;32m> \033[0m"+truncateToWidth(items[i], pt.cols-6))
		} else {
			lines = append(lines, "  "+truncateToWidth(items[i], pt.cols-4))
		}
	}
	if pt.block == 0 {
		pt.drawBlock(lines)
	} else {
		pt.redrawBlock(lines)
	}
}

func (pt *pickerTerm) confirm(question, yesLabel, noLabel string) (yes, ok bool) {
	opts := []string{noLabel, yesLabel}
	sel := 0
	pt.hideCursor()
	pt.ensureRoom(2)
	draw := func() {
		lines := []string{"\033[1;33m? " + truncateToWidth(question, pt.cols-2) + "\033[0m"}
		var ob strings.Builder
		for i, o := range opts {
			if i > 0 {
				ob.WriteString("     ")
			}
			if i == sel {
				ob.WriteString("\033[1;32m> " + o + "\033[0m")
			} else {
				ob.WriteString("  " + o)
			}
		}
		lines = append(lines, ob.String())
		if pt.block == 0 {
			pt.drawBlock(lines)
		} else {
			pt.redrawBlock(lines)
		}
	}
	draw()
	for {
		key, err := pt.in.ReadByte()
		if err != nil {
			pt.eraseBlock()
			pt.showCursor()
			return false, false
		}
		switch {
		case key == '\r' || key == '\n':
			pt.eraseBlock()
			pt.showCursor()
			return sel == 1, true
		case key == '\t' || key == ' ':
			sel = 1 - sel
			draw()
		case key == 0x1b:
			if dir, aok := pt.arrowKey(); aok && (dir == 'C' || dir == 'D') {
				sel = 1 - sel
				draw()
			} else {
				pt.eraseBlock()
				pt.showCursor()
				return false, false
			}
		case key == 'q' || key == 'Q' || key == 0x03:
			pt.eraseBlock()
			pt.showCursor()
			return false, false
		}
	}
}

func (pt *pickerTerm) editLine(prompt, initial, notice string) (string, bool) {
	buf := []rune(initial)
	cursor := len(buf)
	pt.showCursor()
	pt.ensureRoom(3)
	draw := func() {
		lines := []string{"\033[1;36m? " + truncateToWidth(prompt, pt.cols-2) + "\033[0m"}
		if notice != "" {
			lines = append(lines, "\033[1;33m"+truncateToWidth(notice, pt.cols-2)+"\033[0m")
		}
		display := truncateToWidth(string(buf), pt.cols-4)
		lines = append(lines, "\033[32m> \033[0m"+display)
		if pt.block == 0 {
			pt.drawBlock(lines)
		} else {
			pt.redrawBlock(lines)
		}
		col := 3 + runeWidths(buf[:cursor])
		if w := 3 + runeWidths([]rune(display)); col > w {
			col = w
		}
		if col > pt.cols {
			col = pt.cols
		}
		fmt.Fprint(pt.out, "\r")
		if col > 1 {
			fmt.Fprintf(pt.out, "\033[%dC", col-1)
		}
	}
	draw()
	for {
		b, err := pt.in.ReadByte()
		if err != nil {
			pt.eraseBlock()
			return string(buf), false
		}
		switch {
		case b == '\r' || b == '\n':
			pt.eraseBlock()
			return string(buf), true
		case b == 0x03 || b == 0x04:
			pt.eraseBlock()
			return "", false
		case b == 0x7f || b == 0x08:
			if cursor > 0 {
				buf = append(buf[:cursor-1], buf[cursor:]...)
				cursor--
			}
		case b == 0x15:
			buf = nil
			cursor = 0
		case b == 0x01:
			cursor = 0
		case b == 0x05:
			cursor = len(buf)
		case b == 0x1b:
			if dir, aok := pt.arrowKey(); aok {
				switch dir {
				case 'C':
					if cursor < len(buf) {
						cursor++
					}
				case 'D':
					if cursor > 0 {
						cursor--
					}
				case 'H':
					cursor = 0
				case 'F':
					cursor = len(buf)
				}
			} else {
				pt.eraseBlock()
				return "", false
			}
		case b < 0x20:
		case b < utf8.RuneSelf:
			buf = insertRune(buf, cursor, rune(b))
			cursor++
		default:
			size := utf8SeqLen(b)
			seq := []byte{b}
			for n := 1; n < size; n++ {
				nb, err := pt.in.ReadByte()
				if err != nil {
					break
				}
				seq = append(seq, nb)
			}
			if r, _ := utf8.DecodeRune(seq); r != utf8.RuneError {
				buf = insertRune(buf, cursor, r)
				cursor++
			}
		}
		draw()
	}
}

func insertRune(buf []rune, i int, r rune) []rune {
	buf = append(buf, 0)
	copy(buf[i+1:], buf[i:])
	buf[i] = r
	return buf
}
