package main

import (
	"fmt"
	"os"
	"strings"
)

func promptSessionName(r *sessionRegistry, defaultName, phone string) string {
	prompt := "Session name"
	if phone != "" {
		prompt = fmt.Sprintf("Session name for %s", phone)
	}
	prompt += fmt.Sprintf(" (Enter = %q)", defaultName)
	notice := ""
	initial := defaultName
	for {
		pt, ok := openPicker()
		if !ok {
			return unusedName(r, defaultName)
		}
		name, confirmed := pt.editLine(prompt, initial, notice)
		pt.close()
		if !confirmed {
			return defaultName
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return defaultName
		}
		if !r.nameTaken(name) {
			return name
		}
		notice = duplicateNotice(r, name)
		initial = ""
	}
}

func unusedName(r *sessionRegistry, base string) string {
	if !r.nameTaken(base) {
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

func duplicateNotice(r *sessionRegistry, name string) string {
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

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func runAuthList() {
	r, err := loadRegistry()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := ensureMigrated(r); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(r.Sessions) == 0 {
		fmt.Fprintln(os.Stderr, "no sessions saved — run `auth` first (QR code or --pair <phone>)")
		return
	}
	pt, ok := openPicker()
	if !ok {
		printAuthTable(r)
		return
	}
	for {
		items := authListRows(r)
		title := fmt.Sprintf("sessions — %d saved", len(r.Sessions))
		idx, key, keep := pt.pickList(title, "   id        name             phone           last used",
			items, "↑/↓ select · e rename · q/Ctrl-C quit")
		if !keep {
			break
		}
		if key == 'e' || key == 'E' {
			editSessionName(pt, r, idx)
		}
	}
	pt.close()
}

func authListRows(r *sessionRegistry) []string {
	rows := make([]string, 0, len(r.Sessions))
	for _, s := range r.Sessions {
		marker := " "
		if s.ID == r.Active {
			marker = "*"
		}
		rows = append(rows, fmt.Sprintf("%s %-8s %-16s %-15s %s",
			marker, s.ID, s.Name, orDash(s.Phone), relativeTime(s.LastUsed)))
	}
	return rows
}

func printAuthTable(r *sessionRegistry) {
	fmt.Fprintln(os.Stderr, "id        name             phone           last used")
	for _, s := range r.Sessions {
		marker := " "
		if s.ID == r.Active {
			marker = "*"
		}
		fmt.Fprintf(os.Stderr, "%s %-8s %-16s %-15s %s\n",
			marker, s.ID, s.Name, orDash(s.Phone), relativeTime(s.LastUsed))
	}
	fmt.Fprintln(os.Stderr, "\n* = active session")
}

func editSessionName(pt *pickerTerm, r *sessionRegistry, idx int) {
	if idx < 0 || idx >= len(r.Sessions) {
		return
	}
	s := &r.Sessions[idx]
	old := s.Name
	notice := ""
	initial := s.Name
	for {
		newName, confirmed := pt.editLine(fmt.Sprintf("Rename session %q — new name", old), initial, notice)
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
		if !r.nameTaken(newName) {
			s.Name = newName
			if err := r.save(); err != nil {
				fmt.Fprintf(os.Stderr, "save failed: %v\n", err)
			}
			return
		}
		notice = duplicateNotice(r, newName)
		initial = ""
	}
}

func runAuthSwitch(args []string) {
	r, err := loadRegistry()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := ensureMigrated(r); err != nil {
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
				if err := r.setActive(s.ID); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				fmt.Fprintln(termOut, "switched to session:", s.Name)
				return
			}
		}
		fmt.Fprintf(os.Stderr, "no session named %q\n", want)
		os.Exit(1)
	}

	switch len(r.Sessions) {
	case 1:
		if err := r.setActive(r.Sessions[0].ID); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(termOut, "only one session — already active:", r.Sessions[0].Name)
	case 2:
		other := &r.Sessions[0]
		if r.Active == other.ID {
			other = &r.Sessions[1]
		}
		if err := r.setActive(other.ID); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(termOut, "switched to session:", other.Name)
	default:
		pt, ok := openPicker()
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
				marker, r.Sessions[i].Name, orDash(r.Sessions[i].Phone)))
		}
		idx, key, keep := pt.pickList("auth switch — choose the active session",
			"   name             phone", items, "↑/↓ select · Enter switch · q/Ctrl-C quit")
		pt.close()
		if !keep || key != '\r' {
			return
		}
		if err := r.setActive(r.Sessions[idx].ID); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(termOut, "switched to session:", r.Sessions[idx].Name)
	}
}

func runDeauth() {
	r, err := loadRegistry()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := ensureMigrated(r); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(r.Sessions) == 0 {
		fmt.Fprintln(os.Stderr, "no sessions saved — nothing to deauth")
		return
	}
	pt, ok := openPicker()
	if !ok {
		fmt.Fprintln(os.Stderr, "deauth needs an interactive terminal")
		printDeauthTable(r)
		return
	}
	var msgs []string
	for {
		items := deauthRows(r)
		idx, key, keep := pt.pickList("deauth — remove a session", "   name             phone",
			items, "↑/↓ select · Enter delete · q/Ctrl-C quit")
		if !keep {
			break
		}
		if key != '\r' {
			continue
		}
		s := r.Sessions[idx]
		yes, confirmed := pt.confirm(fmt.Sprintf("Are you sure you want to delete session %q?", s.Name), "Yes", "No")
		if !confirmed || !yes {
			continue
		}
		if err := r.remove(s.ID); err != nil {
			msgs = append(msgs, fmt.Sprintf("delete failed: %v", err))
			break
		}
		msgs = append(msgs, fmt.Sprintf("session %q deleted", s.Name))
		if len(r.Sessions) == 0 {
			break
		}
	}
	pt.close()
	for _, m := range msgs {
		fmt.Fprintln(termOut, m)
	}
}

func deauthRows(r *sessionRegistry) []string {
	rows := make([]string, 0, len(r.Sessions))
	for _, s := range r.Sessions {
		rows = append(rows, fmt.Sprintf("%-16s %-15s", s.Name, orDash(s.Phone)))
	}
	return rows
}

func printDeauthTable(r *sessionRegistry) {
	fmt.Fprintln(os.Stderr, "name             phone")
	for _, s := range r.Sessions {
		fmt.Fprintf(os.Stderr, "%-16s %-15s\n", s.Name, orDash(s.Phone))
	}
}
