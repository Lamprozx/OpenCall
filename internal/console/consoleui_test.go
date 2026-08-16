package console

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestUI() (*ConsoleUI, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	ui := &ConsoleUI{
		out:    buf,
		rows:   24,
		cols:   80,
		submit: make(chan string, 16),
		quit:   make(chan struct{}),
	}
	ui.enabled = true
	return ui, buf
}

func TestDrawBox(t *testing.T) {
	ui, buf := newTestUI()
	ui.buf = []rune("react 🎉")
	ui.cursor = len(ui.buf)
	ui.drawBoxLocked()
	out := buf.String()
	if !strings.Contains(out, "╭") || !strings.Contains(out, "╰") {
		t.Errorf("box borders missing:\n%q", out)
	}
	if !strings.Contains(out, "react 🎉") {
		t.Errorf("input row missing buffer:\n%q", out)
	}
	if !strings.Contains(out, "#>") {
		t.Errorf("input row missing prompt:\n%q", out)
	}
	if !strings.Contains(out, "\033[23;12H") {
		t.Errorf("cursor not positioned after input:\n%q", out)
	}
}

func TestEditLineOperations(t *testing.T) {
	ui, _ := newTestUI()
	for _, r := range "ab" {
		ui.insert(r)
	}
	ui.cursor = 1
	ui.insert('中')
	if got := string(ui.buf); got != "a中b" {
		t.Errorf("after insert: %q, want a中b", got)
	}
	ui.backspaceLocked()
	if got := string(ui.buf); got != "ab" {
		t.Errorf("after backspace: %q, want ab", got)
	}
	ui.cursor = 0
	ui.deleteLocked()
	if got := string(ui.buf); got != "b" {
		t.Errorf("after delete: %q, want b", got)
	}
}

func TestHistoryNavigation(t *testing.T) {
	ui, _ := newTestUI()
	ui.history = []string{"help", "react 👍"}
	ui.histIdx = len(ui.history)
	ui.buf = []rune("partial")

	ui.historyPrevLocked()
	if got := string(ui.buf); got != "react 👍" {
		t.Errorf("history prev (1): %q, want react 👍", got)
	}
	ui.historyPrevLocked()
	if got := string(ui.buf); got != "help" {
		t.Errorf("history prev (2): %q, want help", got)
	}
	ui.historyNextLocked()
	if got := string(ui.buf); got != "react 👍" {
		t.Errorf("history next: %q, want react 👍", got)
	}
	ui.historyNextLocked()
	if got := string(ui.buf); got != "partial" {
		t.Errorf("history next to draft: %q, want partial", got)
	}
}

func TestSubmitLinePushesHistoryAndClears(t *testing.T) {
	ui, buf := newTestUI()
	ui.buf = []rune("status")
	ui.submitLine()
	select {
	case got := <-ui.submit:
		if got != "status" {
			t.Errorf("submitted %q, want status", got)
		}
	default:
		t.Fatal("no line submitted")
	}
	if len(ui.buf) != 0 || ui.cursor != 0 {
		t.Errorf("buffer not cleared after submit: %q", string(ui.buf))
	}
	if len(ui.history) != 1 || ui.history[0] != "status" {
		t.Errorf("history = %v, want [status]", ui.history)
	}
	if !strings.Contains(buf.String(), "\033[36m#> \033[0m") {
		t.Errorf("input row not redrawn after submit:\n%q", buf.String())
	}
}

func TestWriteScrollsLogsAboveBox(t *testing.T) {
	ui, buf := newTestUI()
	ui.buf = []rune("pending")
	long := strings.Repeat("x", 200)
	if _, err := ui.Write([]byte(long + "\n")); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "\033[21;1H\033[K") {
		t.Errorf("log line not written to scroll row 21:\n%q", out)
	}
	if strings.Contains(out, strings.Repeat("x", 100)) {
		t.Errorf("log line not truncated to terminal width")
	}
	if !strings.Contains(out, "pending") {
		t.Errorf("input box not redrawn after log write:\n%q", out)
	}
}

func TestWritePassthroughWhenDisabled(t *testing.T) {
	buf := &bytes.Buffer{}
	ui := &ConsoleUI{out: buf}
	if _, err := ui.Write([]byte("plain line\n")); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "plain line\n" {
		t.Errorf("disabled UI should pass through, got %q", buf.String())
	}
}

func TestReadLineContextCancel(t *testing.T) {
	ui, _ := newTestUI()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := ui.ReadLine(ctx); ok {
		t.Error("ReadLine should return ok=false when context is cancelled")
	}
}

func TestSubmitNeverDrops(t *testing.T) {
	ui, _ := newTestUI()
	var got atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ui.submit:
				got.Add(1)
			case <-ui.quit:
				return
			}
		}
	}()
	const n = 40
	for i := 0; i < n; i++ {
		ui.buf = []rune(fmt.Sprintf("cmd%d", i))
		ui.cursor = len(ui.buf)
		ui.submitLine()
	}
	deadline := time.After(3 * time.Second)
	for got.Load() < n {
		select {
		case <-deadline:
			t.Fatalf("only %d of %d submitted lines were delivered", got.Load(), n)
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(ui.quit)
	<-done
}

func TestConcurrentWriteAndInsert(t *testing.T) {
	ui, buf := newTestUI()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			if _, err := ui.Write([]byte("spam log line number here\n")); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			ui.insert('x')
		}
	}()
	wg.Wait()
	ui.mu.Lock()
	ui.drawBoxLocked()
	ui.mu.Unlock()
	out := buf.String()
	if !strings.Contains(out, "╭") || !strings.Contains(out, "╰") {
		t.Fatalf("box borders missing after concurrent writes:\n%q", out)
	}
	if !strings.Contains(out, "spam log line number here") {
		t.Errorf("log lines missing after concurrent activity")
	}
}

func runKeys(ui *ConsoleUI, input string) {
	br := bufio.NewReader(bytes.NewBufferString(input))
	var seq [8]byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			return
		}
		if !ui.dispatchKey(b, br, &seq) {
			return
		}
	}
}

func TestDispatchArrowKeysHistory(t *testing.T) {
	ui, _ := newTestUI()
	ui.history = []string{"help", "react"}
	ui.histIdx = len(ui.history)
	var seq [8]byte

	check := func(keys string, want string) {
		t.Helper()
		br := bufio.NewReader(bytes.NewBufferString(keys))
		if !ui.dispatchKey(0x1b, br, &seq) {
			t.Fatal("dispatchKey stopped unexpectedly")
		}
		if got := string(ui.buf); got != want {
			t.Errorf("%q: buf = %q, want %q", keys, got, want)
		}
	}
	check("[A", "react")
	check("[A", "help")
	check("[B", "react")
	check("[B", "")
}

func TestArrowBurstLeavesNoLiteralText(t *testing.T) {
	ui, _ := newTestUI()
	burst := "\x1b[A\x1b[B\x1b[A\x1b[A\x1b[B\x1b[B\x1b[A\x1b[B\x1b[A\x1b[B"
	runKeys(ui, burst)
	if s := string(ui.buf); strings.ContainsAny(s, "[]") {
		t.Fatalf("swipe burst leaked literal escape text into input: %q", s)
	}
	if s := string(ui.buf); s != "" {
		t.Fatalf("swipe burst should leave input empty, got %q", s)
	}
}

func TestDispatchSS3Arrows(t *testing.T) {
	ui, _ := newTestUI()
	ui.history = []string{"help", "react"}
	ui.histIdx = len(ui.history)
	br := bufio.NewReader(bytes.NewBufferString("OA"))
	var seq [8]byte
	if !ui.dispatchKey(0x1b, br, &seq) {
		t.Fatal("dispatchKey stopped unexpectedly")
	}
	if got := string(ui.buf); got != "react" {
		t.Fatalf("SS3 Up (ESC O A) should recall the newest history entry, got %q", got)
	}
}

func TestDispatchControlStopKeys(t *testing.T) {
	ui, _ := newTestUI()
	var seq [8]byte
	if ui.dispatchKey(0x03, bufio.NewReader(bytes.NewBufferString("")), &seq) {
		t.Error("Ctrl-C should stop the console (return false)")
	}
	if ui.dispatchKey(0x04, bufio.NewReader(bytes.NewBufferString("")), &seq) {
		t.Error("Ctrl-D should stop the console (return false)")
	}
	ui.buf = []rune("hello")
	ui.cursor = 2
	if !ui.dispatchKey(0x01, bufio.NewReader(bytes.NewBufferString("")), &seq) {
		t.Error("Ctrl-A should keep going (return true)")
	}
	if ui.cursor != 0 {
		t.Errorf("Ctrl-A should move the cursor home, got %d", ui.cursor)
	}
}

func TestDispatchLiteralBrackets(t *testing.T) {
	ui, _ := newTestUI()
	runKeys(ui, "[A]B")
	if got := string(ui.buf); got != "[A]B" {
		t.Fatalf("literal brackets should type normally, got %q", got)
	}
}

func TestMeterRenderedOnBorder(t *testing.T) {
	ui, buf := newTestUI()
	ui.SetMeter(func() float32 { return 1.0 }, func() float32 { return 1.0 })
	ui.mu.Lock()
	ui.drawBoxLocked()
	ui.mu.Unlock()
	out := buf.String()
	if !strings.Contains(out, "\033[22;") || !strings.Contains(out, "🔊") {
		t.Fatalf("meter not drawn on the top border row:\n%q", out)
	}
	if !strings.Contains(out, strings.Repeat("█", 12)) {
		t.Errorf("full-scale meter should light all 12 segments")
	}

	ui.SetMeter(nil, nil)
	buf.Reset()
	ui.mu.Lock()
	ui.drawBoxLocked()
	ui.mu.Unlock()
	if strings.Contains(buf.String(), "🔊") {
		t.Errorf("meter bar should not render after SetMeter(nil,nil):\n%q", buf.String())
	}
}

func TestLogRingBounds(t *testing.T) {
	ui, buf := newTestUI()
	for i := 0; i < maxLogLines+50; i++ {
		ui.appendLogLocked(fmt.Sprintf("line %d", i))
	}
	if len(ui.logLines) != maxLogLines {
		t.Fatalf("log ring = %d lines, want %d", len(ui.logLines), maxLogLines)
	}
	ui.mu.Lock()
	ui.redrawLocked()
	ui.mu.Unlock()
	out := buf.String()
	if !strings.Contains(out, "line 549") {
		t.Errorf("newest ring line missing after redraw")
	}
	if strings.Contains(out, "line 0") {
		t.Errorf("oldest ring line should have been evicted")
	}
	if !strings.Contains(out, "╭") || !strings.Contains(out, "╰") {
		t.Errorf("box borders missing after redrawLocked")
	}
}

func TestRuneWidth(t *testing.T) {
	cases := map[rune]int{
		'a': 1, '1': 1, ' ': 1,
		'中': 2, '한': 2, '🎉': 2, '→': 1,
	}
	for r, want := range cases {
		if got := runeWidth(r); got != want {
			t.Errorf("runeWidth(%q) = %d, want %d", r, got, want)
		}
	}
}

func TestRuneWidths(t *testing.T) {
	if got := runeWidths([]rune("ab中🎉")); got != 6 {
		t.Errorf("runeWidths(ab中🎉) = %d, want 6", got)
	}
}

func TestTruncateToWidth(t *testing.T) {
	cases := []struct {
		in   string
		maxW int
		want string
	}{
		{"short", 20, "short"},
		{"0123456789", 12, "0123456789"},
		{"0123456789", 10, "0123456789"},
		{"0123456789", 6, "01234…"},
		{"0123456789", 5, "0123…"},
		{"ab中cd", 4, "ab…"},
		{"中abc", 3, "中…"},
		{"", 10, ""},
		{"x", 0, ""},
		{"abcd", 1, "…"},
	}
	for _, c := range cases {
		if got := truncateToWidth(c.in, c.maxW); got != c.want {
			t.Errorf("truncateToWidth(%q, %d) = %q, want %q", c.in, c.maxW, got, c.want)
		}
	}
}
