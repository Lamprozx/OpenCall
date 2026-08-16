package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestNoiseFilter(t *testing.T) {
	var out bytes.Buffer
	f := &noiseFilter{out: &out}

	noise := []string{
		`13:57:16.000 WRN Ignoring message ACE18C447CE2B1AE45188B1F772021F9 from 231975663173839@lid in status@broadcast: failed to decrypt prekey message sublogger=wa`,
		`13:57:31.000 ERR Failed to save push name of 40694848745649@lid in device store: database is locked (5) (SQLITE_BUSY) sublogger=wa`,
		`13:58:01.000 WRN Error decrypting message ACAA7FDC597C1022A3AEEC85F7372A43 from 205149482000477@lid: no sender key for 205149482000477_1:0 sublogger=wa`,
		`13:58:06.000 WRN Failed to delete all identities of 55435830775875@lid from store after identity change: database is locked (5) (SQLITE_BUSY) sublogger=wa`,
		`13:58:11.000 WRN Node handling took 5.00833732s for <message edit="7" from="status@broadcast"> sublogger=wa`,
	}
	for _, n := range noise {
		if _, err := f.Write([]byte(n + "\n")); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	keep := []string{
		`13:57:17.000 INF call placed — media starts when the peer answers call_id=D6D25526E1311729009E3EB3491FEEC5 target=6289528889669 video=false`,
		`13:57:18.000 INF relay allocation arrived in call ack call_id=D6D25526E1311729009E3EB3491FEEC5`,
		`13:57:24.000 INF media ready — streaming now`,
		`13:57:25.000 INF streaming video file as camera access_units=305`,
		`13:57:26.000 INF first video RTP sent to relay, outbound video flowing`,
		`13:57:30.000 WRN still waiting for the peer to answer — if the target shows 'connecting', they haven't picked up yet phase=connecting`,
		`13:57:31.000 INF call ended reason=user_ended`,
	}
	for _, k := range keep {
		if _, err := f.Write([]byte(k + "\n")); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	got := out.String()
	for _, n := range noise {
		if strings.Contains(got, n) {
			t.Errorf("noise line was not filtered:\n%s", n)
		}
	}
	for _, k := range keep {
		if !strings.Contains(got, k) {
			t.Errorf("useful line was filtered:\n%s", k)
		}
	}
}

func TestNoiseFilterPartialLines(t *testing.T) {
	var out bytes.Buffer
	f := &noiseFilter{out: &out}
	chunk := `failed to decrypt ` + strings.Repeat("x", 500)
	if _, err := f.Write([]byte(chunk[:100])); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(chunk[100:] + "\n")); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("expected buffered noise line to be dropped, got %q", out.String())
	}
}
