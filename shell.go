package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Shell holds interpreter state: variables, aliases, history and the
// standard streams (which tests and command substitution can redirect).
type Shell struct {
	vars       map[string]string // shell-local variables (not exported)
	aliases    map[string]string
	history    []string
	lastStatus int
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
	exitCode   int
	exiting    bool
}

// ErrExit is returned by Run when the exit builtin was executed.
var ErrExit = errors.New("exit")

// NewShell creates a shell bound to the given streams.
func NewShell(stdin io.Reader, stdout, stderr io.Writer) *Shell {
	return &Shell{
		vars:    map[string]string{},
		aliases: map[string]string{},
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
	}
}

// child makes a shell sharing variables and aliases but writing to w
// (used for command substitution).
func (sh *Shell) child(w io.Writer) *Shell {
	return &Shell{
		vars:       sh.vars,
		aliases:    sh.aliases,
		lastStatus: sh.lastStatus,
		stdin:      sh.stdin,
		stdout:     w,
		stderr:     sh.stderr,
	}
}

func (sh *Shell) getVar(name string) string {
	if v, ok := sh.vars[name]; ok {
		return v
	}
	return os.Getenv(name)
}

// Run parses and executes one or more lines. Returns ErrExit after "exit".
func (sh *Shell) Run(input string) error {
	tokens, err := Lex(input)
	if err != nil {
		sh.lastStatus = 2
		return err
	}
	items, err := Parse(tokens)
	if err != nil {
		sh.lastStatus = 2
		return err
	}
	// && and || are left-associative: each pipeline runs or is skipped based
	// on the status so far and the operator that precedes it.
	prevOp := ";"
	for _, item := range items {
		skip := (prevOp == "&&" && sh.lastStatus != 0) || (prevOp == "||" && sh.lastStatus == 0)
		if !skip {
			sh.lastStatus = sh.runPipeline(item.Pipeline)
			if sh.exiting {
				return ErrExit
			}
		}
		prevOp = item.Op
	}
	return nil
}

// runPipeline executes commands connected by pipes and returns the exit
// status of the last one.
func (sh *Shell) runPipeline(pipe *Pipeline) int {
	cmds := pipe.Commands
	if len(cmds) == 1 && !pipe.Background {
		return sh.runCommand(cmds[0], sh.stdin, sh.stdout, sh.stderr)
	}

	var wg sync.WaitGroup
	statuses := make([]int, len(cmds))
	var input io.Reader = sh.stdin
	for i, cmd := range cmds {
		var output io.Writer = sh.stdout
		var pr *os.File
		var pw *os.File
		if i < len(cmds)-1 {
			var err error
			pr, pw, err = os.Pipe()
			if err != nil {
				fmt.Fprintln(sh.stderr, "gosh: pipe:", err)
				return 1
			}
			output = pw
		}
		wg.Add(1)
		go func(i int, cmd *Command, in io.Reader, out io.Writer, closeAfter *os.File) {
			defer wg.Done()
			statuses[i] = sh.runCommand(cmd, in, out, sh.stderr)
			if closeAfter != nil {
				closeAfter.Close()
			}
			if c, ok := in.(*os.File); ok && c != os.Stdin {
				c.Close()
			}
		}(i, cmd, input, output, pw)
		input = pr
	}
	if pipe.Background {
		go wg.Wait()
		return 0
	}
	wg.Wait()
	return statuses[len(statuses)-1]
}

// runCommand expands words, applies redirections and runs a builtin or an
// external program with the given streams.
func (sh *Shell) runCommand(cmd *Command, stdin io.Reader, stdout, stderr io.Writer) int {
	words, assignments, err := sh.expandCommand(cmd)
	if err != nil {
		fmt.Fprintln(sh.stderr, "gosh:", err)
		return 1
	}

	// apply redirections
	var toClose []io.Closer
	defer func() {
		for _, c := range toClose {
			c.Close()
		}
	}()
	for _, r := range cmd.Redirects {
		target := ""
		if r.Target != "" {
			fields, err := sh.expandWord(r.Target)
			if err != nil || len(fields) != 1 {
				fmt.Fprintln(sh.stderr, "gosh: ambiguous redirect")
				return 1
			}
			target = fields[0]
		}
		switch r.Op {
		case "<":
			f, err := os.Open(target)
			if err != nil {
				fmt.Fprintf(sh.stderr, "gosh: %s: %v\n", target, unwrapPathErr(err))
				return 1
			}
			toClose = append(toClose, f)
			stdin = f
		case ">", ">>", "2>", "2>>":
			flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
			if strings.HasSuffix(r.Op, ">>") {
				flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
			}
			f, err := os.OpenFile(target, flags, 0o644)
			if err != nil {
				fmt.Fprintf(sh.stderr, "gosh: %s: %v\n", target, unwrapPathErr(err))
				return 1
			}
			toClose = append(toClose, f)
			if strings.HasPrefix(r.Op, "2") {
				stderr = f
			} else {
				stdout = f
			}
		case "2>&1":
			stderr = stdout
		}
	}

	// bare assignments set shell variables
	if len(words) == 0 {
		for _, a := range assignments {
			name, value, _ := cut(a, "=")
			sh.vars[name] = value
			if _, exported := os.LookupEnv(name); exported {
				os.Setenv(name, value)
			}
		}
		return 0
	}

	if builtin, ok := builtins[words[0]]; ok {
		return builtin(sh, words[1:], stdin, stdout, stderr)
	}
	return sh.runExternal(words, assignments, stdin, stdout, stderr)
}

// expandCommand resolves aliases and expands every word, separating leading
// NAME=value assignments from the command itself.
func (sh *Shell) expandCommand(cmd *Command) (words, assignments []string, err error) {
	raw := cmd.Words
	if len(raw) > 0 {
		if alias, ok := sh.aliases[raw[0]]; ok {
			tokens, lerr := Lex(alias)
			if lerr != nil {
				return nil, nil, lerr
			}
			var aliasWords []string
			for _, t := range tokens {
				if t.Kind == TokWord {
					aliasWords = append(aliasWords, t.Text)
				}
			}
			raw = append(aliasWords, raw[1:]...)
		}
	}
	for _, w := range raw {
		if len(words) == 0 && isAssignment(w) {
			name, value, _ := cut(w, "=")
			fields, e := sh.expandWord(value)
			if e != nil {
				return nil, nil, e
			}
			assignments = append(assignments, name+"="+strings.Join(fields, " "))
			continue
		}
		fields, e := sh.expandWord(w)
		if e != nil {
			return nil, nil, e
		}
		words = append(words, fields...)
	}
	return words, assignments, nil
}

func isAssignment(w string) bool {
	eq := strings.IndexByte(w, '=')
	if eq <= 0 {
		return false
	}
	for i := 0; i < eq; i++ {
		if !isVarChar(w[i]) {
			return false
		}
	}
	return isVarStart(w[0])
}

func (sh *Shell) runExternal(words, assignments []string, stdin io.Reader, stdout, stderr io.Writer) int {
	path, err := exec.LookPath(words[0])
	if err != nil {
		fmt.Fprintf(sh.stderr, "gosh: %s: command not found\n", words[0])
		return 127
	}
	c := exec.Command(path, words[1:]...)
	c.Stdin = stdin
	c.Stdout = stdout
	c.Stderr = stderr
	c.Env = os.Environ()
	for _, a := range assignments {
		c.Env = append(c.Env, a)
	}
	if err := c.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(sh.stderr, "gosh: %s: %v\n", words[0], err)
		return 126
	}
	return 0
}

func unwrapPathErr(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
