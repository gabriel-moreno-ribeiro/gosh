// gosh is a small POSIX-style shell written from scratch in Go.
//
//	gosh                 interactive prompt
//	gosh script.sh       run a script
//	gosh -c "cmd | cmd"  run one command line
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
)

func main() {
	sh := NewShell(os.Stdin, os.Stdout, os.Stderr)

	if len(os.Args) >= 3 && os.Args[1] == "-c" {
		if err := sh.Run(strings.Join(os.Args[2:], " ")); err != nil && err != ErrExit {
			fmt.Fprintln(os.Stderr, "gosh:", err)
			os.Exit(2)
		}
		if sh.exiting {
			os.Exit(sh.exitCode)
		}
		os.Exit(sh.lastStatus)
	}

	if len(os.Args) >= 2 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "gosh: %s: %v\n", os.Args[1], unwrapPathErr(err))
			os.Exit(127)
		}
		defer f.Close()
		os.Exit(sh.runScript(newScanner(f)))
	}

	// interactive: ignore Ctrl-C in the shell itself (children still get it)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	go func() {
		for range sigs {
			fmt.Fprint(os.Stdout, "\n")
			fmt.Fprint(os.Stdout, prompt(sh))
		}
	}()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(os.Stdout, prompt(sh))
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintln(os.Stdout)
			os.Exit(sh.lastStatus)
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		sh.history = append(sh.history, line)
		if err := sh.Run(line); err != nil {
			if err == ErrExit {
				os.Exit(sh.exitCode)
			}
			fmt.Fprintln(os.Stderr, "gosh:", err)
		}
	}
}

func prompt(sh *Shell) string {
	wd, err := os.Getwd()
	if err != nil {
		wd = "?"
	}
	if home := sh.home(); home != "" && strings.HasPrefix(wd, home) {
		wd = "~" + wd[len(home):]
	}
	status := ""
	if sh.lastStatus != 0 {
		status = fmt.Sprintf(" [%d]", sh.lastStatus)
	}
	return fmt.Sprintf("%s%s $ ", filepath.ToSlash(wd), status)
}
