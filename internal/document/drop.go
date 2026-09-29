package document

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/NimbleMarkets/ntcharts-svg/svg"
)

const maxDropBytes = 64 << 10
const maxDropFiles = 256

// ParseDrop extracts file paths from a bracketed paste. Terminals deliver a
// drop as pasted text: one path per line, or several shell-quoted on one line.
// Text that does not read as paths yields nothing.
func ParseDrop(text string) []string {
	if len(text) > maxDropBytes {
		return nil
	}
	var paths []string
	// Terminals commonly send a pasted newline as a carriage return.
	for _, line := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' }) {
		line = strings.TrimSpace(line)
		if line == "" || strings.ContainsFunc(line, func(r rune) bool { return r != '\t' && (r < ' ' || r == 0x7f) }) {
			continue
		}
		// Some terminals paste names containing spaces without quoting them.
		if path, ok := dropPath(line); ok && regularFile(path) {
			paths = append(paths, path)
			continue
		}
		words, ok := shellWords(line)
		if !ok {
			continue
		}
		found := make([]string, 0, len(words))
		for _, word := range words {
			path, ok := dropPath(word)
			if !ok || (!pathLike(word) && !regularFile(path)) {
				found = nil
				break
			}
			found = append(found, path)
		}
		paths = append(paths, found...)
	}
	if len(paths) > maxDropFiles {
		return nil
	}
	return paths
}

func pathLike(s string) bool {
	for _, prefix := range []string{"/", "./", "../", "~/", "file://"} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func dropPath(s string) (string, bool) {
	if strings.HasPrefix(s, "file://") {
		u, err := url.Parse(s)
		if err != nil || (u.Host != "" && u.Host != "localhost") || !strings.HasPrefix(u.Path, "/") {
			return "", false
		}
		return filepath.FromSlash(u.Path), true
	}
	if strings.Contains(s, "://") {
		return "", false
	}
	if rest, ok := strings.CutPrefix(s, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		return filepath.Join(home, rest), true
	}
	return s, true
}

// shellWords splits a line as a POSIX shell would, without expansion.
func shellWords(line string) ([]string, bool) {
	var words []string
	var word strings.Builder
	var quote rune
	inWord, escaped := false, false
	for _, r := range line {
		switch {
		case escaped:
			if quote == '"' && !strings.ContainsRune("\"\\$`", r) {
				word.WriteRune('\\')
			}
			word.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped, inWord = true, true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if escaped || quote != 0 {
		return nil, false
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, true
}

// Skipped words a passed-over input the same way for arguments and drops.
func Skipped(path string, err error) string {
	return fmt.Sprintf("gloss: %s: %s (skipped)", svg.SanitizeForTerminal(path), SkipReason(err))
}

// SkipReason says briefly why an input cannot be shown.
func SkipReason(err error) string {
	reason := err.Error()
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		reason = pathErr.Err.Error() // The message already names the path.
	}
	if errors.Is(err, ErrUnsupported) {
		reason = "unsupported format"
	}
	return reason
}

// ProbeFS is Probe for an embedded filesystem; nil probes the host.
func ProbeFS(files fs.FS, path, forced string) (string, error) {
	if files == nil {
		return Probe(path, forced)
	}
	data, err := readFileFrom(files, path)
	if err != nil {
		return "", err
	}
	return Detect(path, data, forced)
}
