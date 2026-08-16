package app

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

var errEmptyGroupID = errors.New("empty group id")

func errInvalidGroupID(raw string, cause ...error) error {
	if len(cause) > 0 && cause[0] != nil {
		return fmt.Errorf("invalid group id %q: %w", raw, cause[0])
	}
	return fmt.Errorf("invalid group id %q (use a numeric id or a full @g.us JID)", raw)
}

// BareGroupIDRe matches a numeric WhatsApp group id (no @g.us suffix).
var BareGroupIDRe = regexp.MustCompile(`^[0-9-]+$`)

// BoolFlags are flags that never consume a following value, used by
// ReorderArgs to keep flags and positionals parseable by flag.FlagSet.
var BoolFlags = map[string]bool{
	"-auto-answer": true, "--auto-answer": true,
	"-video": true, "--video": true,
	"-qr": true, "--qr": true,
	"-help": true, "--help": true,
	"-h":      true,
	"-stream": true, "--stream": true,
	"-reverse": true, "--reverse": true,
}

// ReorderArgs moves flags (and their values) ahead of positional arguments so
// Go's flag package can parse them regardless of user ordering. `--` terminates
// flag processing; everything after it stays positional.
func ReorderArgs(args []string) []string {
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			flags = append(flags, a)
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			if !BoolFlags[a] && !strings.Contains(a, "=") {
				if i+1 < len(args) {
					i++
					flags = append(flags, args[i])
				} else {
					flags = append(flags, "")
				}
			}
		} else {
			positionals = append(positionals, a)
		}
	}
	return append(flags, positionals...)
}

// NormalizeGroupID converts a bare numeric group id or a full @g.us JID into a
// canonical non-AD group JID string.
func NormalizeGroupID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errEmptyGroupID
	}
	if !strings.Contains(raw, "@") {
		if !BareGroupIDRe.MatchString(raw) {
			return "", errInvalidGroupID(raw)
		}
		return types.NewJID(raw, types.GroupServer).ToNonAD().String(), nil
	}
	jid, err := types.ParseJID(raw)
	if err != nil {
		return "", errInvalidGroupID(raw, err)
	}
	return jid.ToNonAD().String(), nil
}

// GroupJIDsEqual reports whether two group ids normalize to the same JID.
func GroupJIDsEqual(a, b string) bool {
	na, errA := NormalizeGroupID(a)
	nb, errB := NormalizeGroupID(b)
	if errA != nil || errB != nil {
		return false
	}
	return na == nb
}
