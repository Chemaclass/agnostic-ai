// Command trim-leads ranks the moved lines of a changed vendor page against
// a target's claims with trim-1, for scripts/jev-triage.sh. It lives in its
// own module so the release binary never links the model.
//
// Each stdin line is a job: an id, a claims file (one claim per line), and a
// text file (a .delta part, moved words inside [-...-] and {+...+}). For
// each claim it scores the target's other claims and the text's lines with
// the claim as the query, and prints "id, claim line, rank, probability"
// when the best moved line ranks within the cut. The other claims are the
// baseline a moved line must beat: a .delta part is often a single line.
// The cut is -top, or one line in five for a target with few claims, so
// unrelated text cannot outrank them all by default.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	trim "github.com/caveman-ai/trim-1"
)

func main() {
	top := flag.Int("top", 10, "keep a claim when a moved line ranks this high or higher")
	flag.Parse()
	if err := run(os.Stdin, os.Stdout, *top); err != nil {
		fmt.Fprintf(os.Stderr, "trim-leads: %v\n", err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer, top int) error {
	w := bufio.NewWriter(out)
	jobs := bufio.NewScanner(in)
	jobs.Buffer(make([]byte, 64<<10), 1<<20)
	for jobs.Scan() {
		f := strings.Split(jobs.Text(), "\t")
		if len(f) != 3 {
			return fmt.Errorf("job %q: want id, claims file, text file", jobs.Text())
		}
		claims, err := readLines(f[1])
		if err != nil {
			return err
		}
		text, err := readLines(f[2])
		if err != nil {
			return err
		}
		moved := movedLines(text)
		cut := min(top, (nonBlank(claims)+5)/5)
		for i, claim := range claims {
			if strings.TrimSpace(claim) == "" {
				continue
			}
			rank, p, err := movedRank(claims, claim, text, moved)
			if err != nil {
				return err
			}
			if rank == 0 || rank > cut {
				continue
			}
			if _, err := fmt.Fprintf(w, "%s\t%d\t%d\t%.2f\n", f[0], i+1, rank, p); err != nil {
				return err
			}
		}
	}
	if err := jobs.Err(); err != nil {
		return err
	}
	return w.Flush()
}

func nonBlank(lines []string) int {
	n := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

func readLines(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n"), nil
}

// movedLines marks the lines holding a change. A "# whitespace" delta is a
// line diff with no word markers, so every line after its header counts.
func movedLines(text []string) []bool {
	whitespace := len(text) > 0 && strings.HasPrefix(text[0], "# whitespace")
	moved := make([]bool, len(text))
	for i, line := range text {
		moved[i] = strings.Contains(line, "{+") || strings.Contains(line, "[-") || whitespace && i > 0
	}
	return moved
}

// movedRank returns the rank, 1 for the top, of the best moved line among
// the other claims and the text's lines for query, and its keep
// probability. It returns 0 when no moved line scores.
func movedRank(claims []string, query string, text []string, moved []bool) (int, float64, error) {
	context := make([]string, 0, len(claims)+len(text))
	for _, c := range claims {
		if c != query {
			context = append(context, c)
		}
	}
	first := len(context)
	context = append(context, text...)
	_, scores, err := trim.Score(strings.Join(context, "\n"), query)
	if err != nil {
		return 0, 0, err
	}
	if len(scores) != len(context) {
		return 0, 0, errors.New("trim-1 scored a different number of lines than it was given")
	}
	best := 0.0
	for i, m := range moved {
		if m && scores[first+i] > best {
			best = scores[first+i]
		}
	}
	if best == 0 {
		return 0, 0, nil
	}
	rank := 1
	for _, s := range scores {
		if s > best {
			rank++
		}
	}
	return rank, best, nil
}
