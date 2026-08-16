package session

import (
	"fmt"
	"os"
	"strings"

	"opencall/internal/console"
)

// PromptSessionName interactively asks for a unique session name, falling back
// to defaultName when no interactive terminal is available.
func PromptSessionName(r *Registry, defaultName, phone string) string {
	prompt := "Session name"
	if phone != "" {
		prompt = fmt.Sprintf("Session name for %s", phone)
	}
	prompt += fmt.Sprintf(" (Enter = %q)", defaultName)
	notice := ""
	initial := defaultName
	for {
		pt, ok := console.OpenPicker()
		if !ok {
			return unusedName(r, defaultName)
		}
		name, confirmed := pt.EditLine(prompt, initial, notice)
		pt.Close()
		if !confirmed {
			return defaultName
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return defaultName
		}
		if !r.NameTaken(name) {
			return name
		}
		notice = duplicateNotice(r, name)
		initial = ""
	}
}

func unusedName(r *Registry, base string) string {
	if !r.NameTaken(base) {
		return base
	}
	taken := map[string]bool{}
	for _, s := range r.Sessions {
		taken[strings.ToLower(strings.TrimSpace(s.Name))] = true
	}
	if sug := suggestNames(base, taken, 1); len(sug) > 0 {
		return sug[0]
	}
	return base + "x"
}

func duplicateNotice(r *Registry, name string) string {
	taken := map[string]bool{}
	for _, s := range r.Sessions {
		taken[strings.ToLower(strings.TrimSpace(s.Name))] = true
	}
	sug := suggestNames(name, taken, 3)
	if len(sug) == 0 {
		return fmt.Sprintf("a session named %q already exists — pick a different name", name)
	}
	return fmt.Sprintf("a session named %q already exists — try: %s", name, strings.Join(sug, ", "))
}

// OrDash returns "-" for empty strings.
func OrDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// AuthList runs the interactive `auth list` flow.
func AuthList() {
	r, err := LoadRegistry()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := EnsureMigrated(r); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(r.Sessions) == 0 {
		fmt.Fprintln(os.Stderr, "no sessions saved — run `auth` first (QR code or --pair <phone>)")
		return
	}
	pt, ok := console.OpenPicker()
	if !ok {
		printAuthTable(r)
		return
	}
	for {
		items := authListRows(r)
		title := fmt.Sprintf("sessions — %d saved", len(r.Sessions))
		idx, key, keep := pt.PickList(title, "   id        name             phone           last used",
			items, "↑/↓ select · e rename · q/Ctrl-C quit")
		if !keep {
			break
		}
		if key == 'e' || key == 'E' {
			editSessionName(pt, r, idx)
		}
	}
	pt.Close()
}

func authListRows(r *Registry) []string {
	rows := make([]string, 0, len(r.Sessions))
	for _, s := range r.Sessions {
		marker := " "
		if s.ID == r.Active {
			marker = "*"
		}
		rows = append(rows, fmt.Sprintf("%s %-8s %-16s %-15s %s",
			marker, s.ID, s.Name, OrDash(s.Phone), relativeTime(s.LastUsed)))
	}
	return rows
}

func printAuthTable(r *Registry) {
	fmt.Fprintln(os.Stderr, "id        name             phone           last used")
	for _, s := range r.Sessions {
		marker := " "
		if s.ID == r.Active {
			marker = "*"
		}
		fmt.Fprintf(os.Stderr, "%s %-8s %-16s %-15s %s\n",
			marker, s.ID, s.Name, OrDash(s.Phone), relativeTime(s.LastUsed))
	}
	fmt.Fprintln(os.Stderr, "\n* = active session")
}

func editSessionName(pt *console.Picker, r *Registry, idx int) {
	if idx < 0 || idx >= len(r.Sessions) {
		return
	}
	s := &r.Sessions[idx]
	old := s.Name
	notice := ""
	initial := s.Name
	for {
		newName, confirmed := pt.EditLine(fmt.Sprintf("Rename session %q — new name", old), initial, notice)
		if !confirmed {
			return
		}
		newName = strings.TrimSpace(newName)
		if newName == "" {
			return
		}
		if strings.EqualFold(newName, s.Name) {
			return
		}
		if !r.NameTaken(newName) {
			s.Name = newName
			if err := r.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "save failed: %v\n", err)
			}
			return
		}
		notice = duplicateNotice(r, newName)
		initial = ""
	}
}

// AuthSwitch runs the interactive `auth switch` flow (or a direct name switch).
func AuthSwitch(args []string) {
	r, err := LoadRegistry()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := EnsureMigrated(r); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(r.Sessions) == 0 {
		fmt.Fprintln(os.Stderr, "no sessions saved — run `auth` first (QR code or --pair <phone>)")
		return
	}
	if len(args) > 0 {
		want := strings.TrimSpace(args[0])
		for i := range r.Sessions {
			s := &r.Sessions[i]
			if s.ID == want || strings.EqualFold(s.Name, want) {
				if err := r.SetActive(s.ID); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				fmt.Fprintln(console.TermOut, "switched to session:", s.Name)
				return
			}
		}
		fmt.Fprintf(os.Stderr, "no session named %q\n", want)
		os.Exit(1)
	}

	switch len(r.Sessions) {
	case 1:
		if err := r.SetActive(r.Sessions[0].ID); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(console.TermOut, "only one session — already active:", r.Sessions[0].Name)
	case 2:
		other := &r.Sessions[0]
		if r.Active == other.ID {
			other = &r.Sessions[1]
		}
		if err := r.SetActive(other.ID); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(console.TermOut, "switched to session:", other.Name)
	default:
		pt, ok := console.OpenPicker()
		if !ok {
			fmt.Fprintln(os.Stderr, "auth switch needs an interactive terminal when 3+ sessions exist (or pass a session name: `auth switch <name>`)")
			printAuthTable(r)
			return
		}
		items := make([]string, 0, len(r.Sessions))
		for i := range r.Sessions {
			marker := " "
			if r.Sessions[i].ID == r.Active {
				marker = "*"
			}
			items = append(items, fmt.Sprintf("%s %-16s %-15s",
				marker, r.Sessions[i].Name, OrDash(r.Sessions[i].Phone)))
		}
		idx, key, keep := pt.PickList("auth switch — choose the active session",
			"   name             phone", items, "↑/↓ select · Enter switch · q/Ctrl-C quit")
		pt.Close()
		if !keep || key != '\r' {
			return
		}
		if err := r.SetActive(r.Sessions[idx].ID); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(console.TermOut, "switched to session:", r.Sessions[idx].Name)
	}
}

// Deauth runs the interactive `deauth` (delete session) flow.
func Deauth() {
	r, err := LoadRegistry()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := EnsureMigrated(r); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(r.Sessions) == 0 {
		fmt.Fprintln(os.Stderr, "no sessions saved — nothing to deauth")
		return
	}
	pt, ok := console.OpenPicker()
	if !ok {
		fmt.Fprintln(os.Stderr, "deauth needs an interactive terminal")
		printDeauthTable(r)
		return
	}
	var msgs []string
	for {
		items := deauthRows(r)
		idx, key, keep := pt.PickList("deauth — remove a session", "   name             phone",
			items, "↑/↓ select · Enter delete · q/Ctrl-C quit")
		if !keep {
			break
		}
		if key != '\r' {
			continue
		}
		s := r.Sessions[idx]
		yes, confirmed := pt.Confirm(fmt.Sprintf("Are you sure you want to delete session %q?", s.Name), "Yes", "No")
		if !confirmed || !yes {
			continue
		}
		if err := r.Remove(s.ID); err != nil {
			msgs = append(msgs, fmt.Sprintf("delete failed: %v", err))
			break
		}
		msgs = append(msgs, fmt.Sprintf("session %q deleted", s.Name))
		if len(r.Sessions) == 0 {
			break
		}
	}
	pt.Close()
	for _, m := range msgs {
		fmt.Fprintln(console.TermOut, m)
	}
}

func deauthRows(r *Registry) []string {
	rows := make([]string, 0, len(r.Sessions))
	for _, s := range r.Sessions {
		rows = append(rows, fmt.Sprintf("%-16s %-15s", s.Name, OrDash(s.Phone)))
	}
	return rows
}

func printDeauthTable(r *Registry) {
	fmt.Fprintln(os.Stderr, "name             phone")
	for _, s := range r.Sessions {
		fmt.Fprintf(os.Stderr, "%-16s %-15s\n", s.Name, OrDash(s.Phone))
	}
}
