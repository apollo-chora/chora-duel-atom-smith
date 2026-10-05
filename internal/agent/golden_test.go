package agent

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// updateGolden regenerates the testdata/golden_smith.txt file when an
// INTENTIONAL prompt change lands (run: go test ./internal/agent/ -update).
var updateGolden = flag.Bool("update", false, "regenerate golden_smith.txt")

// goldenCtx is the fixed TaskContext whose composed instruction is captured in
// testdata/golden_smith.txt. Any drift here means the smith prompt changed —
// regenerate with -update only when the change is intentional.
var goldenCtx = TaskContext{
	TenantID: "tenant-x",
	UserGCID: "gcid-y",
	Candidates: []Candidate{
		{Index: 0, Question: "What is 2+2?", Options: []string{"3", "4", "5", "6"}},
	},
	SharedTags:    []string{"mathematics"},
	Proficiencies: []int{1200},
	Profiles:      map[string]string{"gcid-y": "intermediate"},
	Count:         3,
}

func TestComposeSmith_goldenByteIdentical(t *testing.T) {
	got := ComposeInstruction(RoleSmith, goldenCtx)
	path := filepath.Join("testdata", "golden_smith.txt")
	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", path, err)
	}
	if got != string(want) {
		t.Errorf("smith prompt drifted from the captured golden — if intentional, regenerate with -update.\n%s", firstDiff(string(want), got))
	}
}

// firstDiff returns a readable byte-level diff for the golden assertion.
func firstDiff(want, got string) string {
	minLen := len(want)
	if len(got) < minLen {
		minLen = len(got)
	}
	for i := range minLen {
		if want[i] != got[i] {
			start := i - 40
			if start < 0 {
				start = 0
			}
			end := i + 40
			if end > minLen {
				end = minLen
			}
			return "first diff at byte " + itoa(i) + ":\n" +
				"  want: ..." + want[start:end] + "...\n" +
				"  got:  ..." + got[start:end] + "..."
		}
	}
	if len(want) != len(got) {
		return "length diff: want=" + itoa(len(want)) + " got=" + itoa(len(got))
	}
	return ""
}

// itoa is a dependency-free int→string for the diff helper (avoids strconv import
// bloat in the test file).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
