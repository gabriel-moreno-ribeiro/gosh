package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Builtin is a command implemented inside the shell. It receives the
// streams so it can take part in pipelines and redirections.
type Builtin func(sh *Shell, args []string, stdin io.Reader, stdout, stderr io.Writer) int

var builtins map[string]Builtin

func init() {
	builtins = map[string]Builtin{
		"cd":      builtinCd,
		"pwd":     builtinPwd,
		"echo":    builtinEcho,
		"exit":    builtinExit,
		"export":  builtinExport,
		"unset":   builtinUnset,
		"env":     builtinEnv,
		"set":     builtinSet,
		"alias":   builtinAlias,
		"unalias": builtinUnalias,
		"history": builtinHistory,
		"type":    builtinType,
		"true":    func(*Shell, []string, io.Reader, io.Writer, io.Writer) int { return 0 },
		"false":   func(*Shell, []string, io.Reader, io.Writer, io.Writer) int { return 1 },
		"source":  builtinSource,
		".":       builtinSource,
		"help":    builtinHelp,
	}
}

func builtinCd(sh *Shell, args []string, _ io.Reader, _, stderr io.Writer) int {
	target := ""
	switch {
	case len(args) == 0:
		target = sh.home()
	case args[0] == "-":
		target = sh.getVar("OLDPWD")
		if target == "" {
			fmt.Fprintln(stderr, "cd: OLDPWD not set")
			return 1
		}
	default:
		target = args[0]
	}
	old, _ := os.Getwd()
	if err := os.Chdir(target); err != nil {
		fmt.Fprintf(stderr, "cd: %s: %v\n", target, unwrapPathErr(err))
		return 1
	}
	now, _ := os.Getwd()
	os.Setenv("OLDPWD", old)
	os.Setenv("PWD", now)
	return 0
}

func builtinPwd(_ *Shell, _ []string, _ io.Reader, stdout, _ io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		return 1
	}
	fmt.Fprintln(stdout, wd)
	return 0
}

func builtinEcho(_ *Shell, args []string, _ io.Reader, stdout, _ io.Writer) int {
	newline := true
	if len(args) > 0 && args[0] == "-n" {
		newline = false
		args = args[1:]
	}
	fmt.Fprint(stdout, strings.Join(args, " "))
	if newline {
		fmt.Fprintln(stdout)
	}
	return 0
}

func builtinExit(sh *Shell, args []string, _ io.Reader, _, stderr io.Writer) int {
	code := sh.lastStatus
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Fprintf(stderr, "exit: %s: numeric argument required\n", args[0])
			code = 2
		} else {
			code = n
		}
	}
	sh.exitCode = code
	sh.exiting = true
	return code
}

func builtinExport(sh *Shell, args []string, _ io.Reader, stdout, _ io.Writer) int {
	if len(args) == 0 {
		env := os.Environ()
		sort.Strings(env)
		for _, e := range env {
			fmt.Fprintln(stdout, "export", e)
		}
		return 0
	}
	for _, a := range args {
		name, value, hasValue := cut(a, "=")
		if !hasValue {
			value = sh.vars[name]
		}
		os.Setenv(name, value)
		delete(sh.vars, name)
	}
	return 0
}

func builtinUnset(sh *Shell, args []string, _ io.Reader, _, _ io.Writer) int {
	for _, name := range args {
		delete(sh.vars, name)
		os.Unsetenv(name)
	}
	return 0
}

func builtinEnv(_ *Shell, _ []string, _ io.Reader, stdout, _ io.Writer) int {
	env := os.Environ()
	sort.Strings(env)
	for _, e := range env {
		fmt.Fprintln(stdout, e)
	}
	return 0
}

func builtinSet(sh *Shell, _ []string, _ io.Reader, stdout, _ io.Writer) int {
	names := make([]string, 0, len(sh.vars))
	for k := range sh.vars {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Fprintf(stdout, "%s=%s\n", k, sh.vars[k])
	}
	return 0
}

func builtinAlias(sh *Shell, args []string, _ io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		names := make([]string, 0, len(sh.aliases))
		for k := range sh.aliases {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			fmt.Fprintf(stdout, "alias %s='%s'\n", k, sh.aliases[k])
		}
		return 0
	}
	status := 0
	for _, a := range args {
		name, value, ok := cut(a, "=")
		if !ok {
			if v, found := sh.aliases[name]; found {
				fmt.Fprintf(stdout, "alias %s='%s'\n", name, v)
			} else {
				fmt.Fprintf(stderr, "alias: %s: not found\n", name)
				status = 1
			}
			continue
		}
		sh.aliases[name] = value
	}
	return status
}

func builtinUnalias(sh *Shell, args []string, _ io.Reader, _, _ io.Writer) int {
	for _, name := range args {
		delete(sh.aliases, name)
	}
	return 0
}

func builtinHistory(sh *Shell, _ []string, _ io.Reader, stdout, _ io.Writer) int {
	for i, line := range sh.history {
		fmt.Fprintf(stdout, "%5d  %s\n", i+1, line)
	}
	return 0
}

func builtinType(sh *Shell, args []string, _ io.Reader, stdout, stderr io.Writer) int {
	status := 0
	for _, name := range args {
		switch {
		case sh.aliases[name] != "":
			fmt.Fprintf(stdout, "%s is aliased to '%s'\n", name, sh.aliases[name])
		case builtins[name] != nil:
			fmt.Fprintf(stdout, "%s is a shell builtin\n", name)
		default:
			if path, err := exec.LookPath(name); err == nil {
				fmt.Fprintf(stdout, "%s is %s\n", name, path)
			} else {
				fmt.Fprintf(stderr, "type: %s: not found\n", name)
				status = 1
			}
		}
	}
	return status
}

func builtinSource(sh *Shell, args []string, _ io.Reader, _, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "source: filename argument required")
		return 2
	}
	f, err := os.Open(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "source: %s: %v\n", args[0], unwrapPathErr(err))
		return 1
	}
	defer f.Close()
	return sh.runScript(newScanner(f))
}

func builtinHelp(_ *Shell, _ []string, _ io.Reader, stdout, _ io.Writer) int {
	names := make([]string, 0, len(builtins))
	for k := range builtins {
		names = append(names, k)
	}
	sort.Strings(names)
	fmt.Fprintln(stdout, "gosh builtins:", strings.Join(names, " "))
	fmt.Fprintln(stdout, "features: pipes | && || ; & redirects < > >> 2> 2>&1 quotes $VAR ${VAR} $? $(cmd) `cmd` ~ globs aliases")
	return 0
}

// runScript executes lines from a scanner until EOF or exit.
func (sh *Shell) runScript(scanner *bufio.Scanner) int {
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if err := sh.Run(line); err != nil {
			if err == ErrExit {
				return sh.exitCode
			}
			fmt.Fprintln(sh.stderr, "gosh:", err)
		}
	}
	return sh.lastStatus
}
