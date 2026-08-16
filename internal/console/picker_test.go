package console

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func testPicker(input string) (*Picker, *bytes.Buffer) {
	var out bytes.Buffer
	return &Picker{
		in:      bufio.NewReader(bytes.NewBufferString(input)),
		out:     &out,
		rows:    24,
		cols:    100,
		restore: nil,
	}, &out
}

func TestPickListArrowsAndEnter(t *testing.T) {
	pt, _ := testPicker("j\r")
	items := []string{"a", "b", "c"}
	idx, key, ok := pt.PickList("title", "header", items, "footer")
	if !ok || key != '\r' || idx != 1 {
		t.Fatalf("got (%d, %q, %v), want (1, '\\r', true)", idx, key, ok)
	}
}

func TestPickListUpAtTop(t *testing.T) {
	pt, _ := testPicker("k\r")
	idx, key, ok := pt.PickList("t", "", []string{"a", "b"}, "f")
	if !ok || key != '\r' || idx != 0 {
		t.Fatalf("got (%d, %q, %v), want (0, '\\r', true)", idx, key, ok)
	}
}

func TestPickListEscapeArrow(t *testing.T) {
	pt, _ := testPicker("\x1b[B\r")
	idx, key, ok := pt.PickList("t", "", []string{"a", "b", "c"}, "f")
	if !ok || key != '\r' || idx != 1 {
		t.Fatalf("got (%d, %q, %v), want (1, '\\r', true)", idx, key, ok)
	}
}

func TestPickListQuitKeys(t *testing.T) {
	for _, in := range []string{"q", "Q", "\x03", "\x1b"} {
		pt, _ := testPicker(in)
		_, _, ok := pt.PickList("t", "", []string{"a", "b"}, "f")
		if ok {
			t.Fatalf("input %q should abort the picker", in)
		}
	}
}

func TestPickListCustomKey(t *testing.T) {
	pt, _ := testPicker("e")
	idx, key, ok := pt.PickList("t", "", []string{"a", "b"}, "f")
	if !ok || key != 'e' || idx != 0 {
		t.Fatalf("got (%d, %q, %v), want (0, 'e', true)", idx, key, ok)
	}
}

func TestPickListRendersTitleAndRows(t *testing.T) {
	pt, out := testPicker("q")
	pt.PickList("my sessions", "header", []string{"row one", "row two"}, "quit hint")
	s := out.String()
	for _, want := range []string{"my sessions", "row one", "row two", "quit hint"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q: %q", want, s)
		}
	}
}

func TestConfirmPointerStartsOnNo(t *testing.T) {
	pt, out := testPicker("\r")
	yes, ok := pt.Confirm("sure?", "Yes", "No")
	if !ok || yes {
		t.Fatalf("Enter with pointer on No should return (false, true), got (%v, %v)", yes, ok)
	}
	if s := out.String(); !strings.Contains(s, "> No") {
		t.Errorf("pointer should be on No: %q", s)
	}
}

func TestConfirmMoveToYes(t *testing.T) {
	pt, _ := testPicker("\t\r")
	yes, ok := pt.Confirm("sure?", "Yes", "No")
	if !ok || !yes {
		t.Fatalf("Tab+Enter should pick Yes, got (%v, %v)", yes, ok)
	}
	pt2, _ := testPicker(" \r")
	yes2, ok2 := pt2.Confirm("sure?", "Yes", "No")
	if !ok2 || !yes2 {
		t.Fatalf("space+Enter should pick Yes, got (%v, %v)", yes2, ok2)
	}
}

func TestConfirmCancel(t *testing.T) {
	for _, in := range []string{"q", "\x03", "\x1b"} {
		pt, _ := testPicker(in)
		if _, ok := pt.Confirm("sure?", "Yes", "No"); ok {
			t.Fatalf("input %q should cancel the confirm dialog", in)
		}
	}
}

func TestEditLineBasic(t *testing.T) {
	pt, _ := testPicker("udin\r")
	got, ok := pt.EditLine("Session name", "", "")
	if !ok || got != "udin" {
		t.Fatalf("got (%q, %v), want (udin, true)", got, ok)
	}
}

func TestEditLinePrefilledAndBackspace(t *testing.T) {
	pt, _ := testPicker("\x7f\x7fx\r")
	got, ok := pt.EditLine("Rename", "abc", "")
	if !ok || got != "ax" {
		t.Fatalf("got (%q, %v), want (ax, true)", got, ok)
	}
}

func TestEditLineEmptySubmitKeepsBuffer(t *testing.T) {
	pt, _ := testPicker("\r")
	got, ok := pt.EditLine("Session name", "default", "")
	if !ok || got != "default" {
		t.Fatalf("got (%q, %v), want (default, true)", got, ok)
	}
}

func TestEditLineEmoji(t *testing.T) {
	pt, _ := testPicker("🎉\r")
	got, ok := pt.EditLine("Nama", "", "")
	if !ok || got != "🎉" {
		t.Fatalf("got (%q, %v), want (🎉, true)", got, ok)
	}
}

func TestEditLineCtrlUClears(t *testing.T) {
	pt, _ := testPicker("hello\x15\r")
	got, ok := pt.EditLine("Nama", "", "")
	if !ok || got != "" {
		t.Fatalf("got (%q, %v), want (\"\", true)", got, ok)
	}
}

func TestEditLineCancel(t *testing.T) {
	for _, in := range []string{"\x03", "\x04", "\x1b"} {
		pt, _ := testPicker(in)
		if _, ok := pt.EditLine("Nama", "", ""); ok {
			t.Fatalf("input %q should cancel EditLine", in)
		}
	}
}

func TestEditLineLeftRightArrows(t *testing.T) {
	pt, _ := testPicker("abc\x1b[D\x1b[DX\r")
	got, ok := pt.EditLine("Nama", "", "")
	if !ok || got != "aXbc" {
		t.Fatalf("got (%q, %v), want (aXbc, true)", got, ok)
	}
}

func TestEditLineShowsNotice(t *testing.T) {
	pt, out := testPicker("\x03")
	pt.EditLine("Session name", "", "a session named \"udin\" already exists — try: udin2, udin123")
	if s := out.String(); !strings.Contains(s, "already exists") {
		t.Errorf("notice should be rendered: %q", s)
	}
}

func TestInsertRune(t *testing.T) {
	got := insertRune([]rune("ac"), 1, 'b')
	if string(got) != "abc" {
		t.Fatalf("insertRune = %q, want abc", string(got))
	}
}

func TestEnsureRoomFitsBelow(t *testing.T) {
	pt, out := testPicker("")
	pt.startRow = 20
	pt.ensureRoom(3)
	if out.Len() != 0 {
		t.Fatalf("fits-below case must emit nothing, got %q", out.String())
	}
	if pt.startRow != 20 {
		t.Fatalf("startRow = %d, want 20", pt.startRow)
	}
}

func TestEnsureRoomScrollsAtBottom(t *testing.T) {
	pt, out := testPicker("")
	pt.startRow = 24
	pt.ensureRoom(3)
	want := "\n\n\x1b[2A"
	if s := out.String(); s != want {
		t.Fatalf("got %q, want %q", s, want)
	}
	if pt.startRow != 22 {
		t.Fatalf("startRow = %d, want 22", pt.startRow)
	}
}

func TestEnsureRoomScrollsGeneral(t *testing.T) {
	pt, out := testPicker("")
	pt.startRow = 20
	pt.ensureRoom(6)
	want := "\x1b[4B\n\x1b[5A"
	if s := out.String(); s != want {
		t.Fatalf("got %q, want %q", s, want)
	}
	if pt.startRow != 19 {
		t.Fatalf("startRow = %d, want 19", pt.startRow)
	}
}

func TestBlockRedrawCursorMath(t *testing.T) {
	pt, out := testPicker("jq")
	pt.PickList("t", "h", []string{"a", "b", "c"}, "f")
	s := out.String()
	if strings.Contains(s, "\x1b[5A") {
		t.Fatalf("redraw/erase must step up (lines-1)=4, not 5: %q", s)
	}
	if n := strings.Count(s, "\x1b[4A"); n < 2 {
		t.Fatalf("expected >= 2 up-4 sequences (redraw + erase), got %d: %q", n, s)
	}
}

func TestBlockEraseClearsEveryLine(t *testing.T) {
	pt, out := testPicker("q")
	pt.PickList("t", "", []string{"a", "b", "c"}, "f")
	s := out.String()
	if n := strings.Count(s, "\x1b[K"); n < 4 {
		t.Fatalf("expected >= 4 line-clears on quit, got %d: %q", n, s)
	}
	if pt.block != 0 {
		t.Fatalf("block should be 0 after quit, got %d", pt.block)
	}
}
