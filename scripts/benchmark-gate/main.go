// Command benchmark-gate checks the lexer and parser allocation benchmarks.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func main() {
	cmd := exec.Command("go", "test", "./lexer", "./parser", "-run", "^$", "-bench", "^Benchmark(Lexer|Parser)$", "-benchmem", "-count=1")
	output, err := cmd.CombinedOutput()
	fmt.Print(string(output))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := checkBenchmarks(string(output)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func checkBenchmarks(output string) error {
	names := [...]string{"BenchmarkLexer-", "BenchmarkParser-"}
	var seen [len(names)]bool
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		for i, name := range names {
			if !strings.HasPrefix(fields[0], name) {
				continue
			}
			if len(fields) < 3 || fields[len(fields)-1] != "allocs/op" {
				return fmt.Errorf("%s: missing allocs/op result", name)
			}
			allocs, err := strconv.Atoi(fields[len(fields)-2])
			if err != nil {
				return fmt.Errorf("%s: invalid allocs/op: %w", name, err)
			}
			if allocs != 0 {
				return fmt.Errorf("%s: %d allocs/op, want 0", name, allocs)
			}
			seen[i] = true
		}
	}
	for i, name := range names {
		if !seen[i] {
			return fmt.Errorf("%s was not reported", strings.TrimSuffix(name, "-"))
		}
	}
	return nil
}
