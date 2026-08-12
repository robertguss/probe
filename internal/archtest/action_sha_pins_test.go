package archtest_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

var usesLine = regexp.MustCompile(`(?m)^\s*uses:\s*([^@\s]+)@([0-9a-fA-F]+)`)

// TestWorkflowActionSHAPins enforces REQ-224: every uses: pin is a 40-char SHA.
func TestWorkflowActionSHAPins(t *testing.T) {
	log := testutil.New(t)
	log.Phase("sha_pins")
	root := repoRoot(t)
	var offenders []string
	scan := func(dir string) {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			// Flag floating tags: uses: org/act@v1 without SHA
			for i, line := range strings.Split(string(b), "\n") {
				trim := strings.TrimSpace(line)
				if !strings.HasPrefix(trim, "uses:") {
					continue
				}
				if strings.Contains(trim, "@") {
					// accept only 40-hex after @
					at := strings.LastIndex(trim, "@")
					pin := strings.TrimSpace(trim[at+1:])
					pin = strings.Split(pin, " ")[0]
					pin = strings.Split(pin, "#")[0]
					pin = strings.TrimSpace(pin)
					if len(pin) != 40 || !isHex(pin) {
						offenders = append(offenders, rel+":"+itoa(i+1)+": "+trim)
					}
				}
			}
			return nil
		})
	}
	scan(filepath.Join(root, ".github", "workflows"))
	// Also generated distribution templates in catalog.
	c, err := catalog.Load()
	if err != nil {
		log.Fail("catalog", err.Error())
	}
	dist, _ := c.Manifest("distribution")
	if dist != nil {
		for _, f := range dist.Files {
			if !strings.Contains(f.Path, "workflows") {
				continue
			}
			raw, err := c.Read(filepath.ToSlash(filepath.Join(dist.UnitDir, f.Source)))
			if err != nil {
				log.Fail("read", err.Error())
			}
			for i, line := range strings.Split(string(raw), "\n") {
				trim := strings.TrimSpace(line)
				if !strings.HasPrefix(trim, "uses:") {
					continue
				}
				at := strings.LastIndex(trim, "@")
				if at < 0 {
					continue
				}
				pin := strings.Fields(strings.TrimSpace(trim[at+1:]))[0]
				pin = strings.Split(pin, "#")[0]
				if len(pin) != 40 || !isHex(pin) {
					offenders = append(offenders, f.Source+":"+itoa(i+1)+": "+trim)
				}
			}
		}
	}
	log.Assert("no_offenders", len(offenders) == 0, 0, len(offenders))
	for _, o := range offenders {
		t.Logf("offending: %s", o)
		log.Fail("pin", o)
	}
	log.PhaseEnd("sha_pins", testutil.OutcomeOK)
}

func isHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
