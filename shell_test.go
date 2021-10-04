package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testCase struct {
	line string
	want string
}

// run executes a command line in a fresh shell and returns stdout, stderr,
// the exit status and any parse/exit error.
func run(t *testing.T, line string) (string, string, int, error) {
	t.Helper()
	var out, errOut strings.Builder
	sh := NewShell(strings.NewReader(""), &out, &errOut)
	err := sh.Run(line)
	return out.String(), errOut.String(), sh.lastStatus, err
}

func stdout(t *testing.T, line string) string {
	t.Helper()
	out, errOut, _, err := run(t, line)
	if err != nil {
		t.Fatalf("%q: unexpected error: %v (stderr %q)", line, err, errOut)
	}
	return out
}

func checkAll(t *testing.T, cases []testCase) {
	t.Helper()
	for _, c := range cases {
		if got := stdout(t, c.line); got != c.want {
			t.Errorf("%q = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestLexOperatorsAndQuotes(t *testing.T) {
	tokens, err := Lex(`echo "a b" 'c d' e\ f | grep x >> out 2> err 2>&1 && true || false; ls & # comment`)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []TokenKind
	var texts []string
	for _, tok := range tokens {
		kinds = append(kinds, tok.Kind)
		texts = append(texts, tok.Text)
	}
	want := []string{"echo", `"a b"`, `'c d'`, `e\ f`, "|", "grep", "x", ">>", "out", "2>", "err", "2>&1", "&&", "true", "||", "false", ";", "ls", "&", ""}
	if strings.Join(texts, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tokens = %q, want %q", texts, want)
	}
	if kinds[1] != TokWord || kinds[4] != TokPipe || kinds[7] != TokRedirect || kinds[12] != TokAnd || kinds[14] != TokOr || kinds[16] != TokSemi || kinds[18] != TokAmp {
		t.Fatalf("unexpected kinds %v", kinds)
	}
}

func TestLexKeepsCommandSubstitutionTogether(t *testing.T) {
	tokens, err := Lex("echo $(echo a b) `echo c d` x")
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 5 || tokens[1].Text != "$(echo a b)" || tokens[2].Text != "`echo c d`" {
		t.Fatalf("tokens = %+v", tokens)
	}
}

func TestLexErrors(t *testing.T) {
	for _, bad := range []string{`echo "unterminated`, `echo 'x`, "echo `x", "echo $(x"} {
		if _, err := Lex(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func mustLex(s string) []Token {
	tokens, _ := Lex(s)
	return tokens
}

func TestParsePipelinesAndLists(t *testing.T) {
	items, err := Parse(mustLex("a b | c < in > out && d || e; f &"))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("got %d items", len(items))
	}
	first := items[0].Pipeline
	if len(first.Commands) != 2 || first.Commands[0].Words[1] != "b" || first.Commands[1].Words[0] != "c" {
		t.Fatalf("pipeline = %+v", first)
	}
	if len(first.Commands[1].Redirects) != 2 || first.Commands[1].Redirects[0].Op != "<" || first.Commands[1].Redirects[1].Target != "out" {
		t.Fatalf("redirects = %+v", first.Commands[1].Redirects)
	}
	if items[0].Op != "&&" || items[1].Op != "||" || items[2].Op != ";" {
		t.Fatalf("ops = %q %q %q", items[0].Op, items[1].Op, items[2].Op)
	}
	if !items[3].Pipeline.Background {
		t.Fatal("last pipeline should be background")
	}
	if _, err := Parse(mustLex("| a")); err == nil {
		t.Fatal("expected syntax error for leading pipe")
	}
	if _, err := Parse(mustLex("a >")); err == nil {
		t.Fatal("expected syntax error for dangling redirect")
	}
}

func TestEchoAndQuoting(t *testing.T) {
	checkAll(t, []testCase{
		{`echo hello   world`, "hello world\n"},
		{`echo "hello   world"`, "hello   world\n"},
		{`echo 'single $HOME'`, "single $HOME\n"},
		{`echo a\ b`, "a b\n"},
		{`echo "quote \" inside"`, "quote \" inside\n"},
		{`echo -n no newline`, "no newline"},
		{`echo 'it'"'"'s'`, "it's\n"},
		{`echo ""`, "\n"},
		{`echo`, "\n"},
		{`echo a;echo b`, "a\nb\n"},
		{"echo one\necho two", "one\ntwo\n"},
		{`echo hi # trailing comment`, "hi\n"},
		{`echo \$notavar`, "$notavar\n"},
		{`echo "price: \$5"`, "price: $5\n"},
	})
}

func TestVariables(t *testing.T) {
	os.Setenv("GOSH_TEST_VAR", "from env")
	defer os.Unsetenv("GOSH_TEST_VAR")
	checkAll(t, []testCase{
		{`echo $GOSH_TEST_VAR`, "from env\n"},
		{`echo "${GOSH_TEST_VAR}!"`, "from env!\n"},
		{`echo ${GOSH_TEST_VAR}x`, "from envx\n"},
		{`X=5; echo $X`, "5\n"},
		{`X=5; echo "$X$X"`, "55\n"},
		{`X="a b"; echo $X`, "a b\n"},
		{`X=hello; echo ${X}world`, "helloworld\n"},
		{`X=1; X=2; echo $X`, "2\n"},
		{`echo $UNDEFINED_VARIABLE_XYZ`, "\n"},
		{`X=hi; unset X; echo "[$X]"`, "[]\n"},
		{`true; echo $?`, "0\n"},
		{`false; echo $?`, "1\n"},
		{`false || echo $?`, "1\n"},
		{`X=abc; export X; printenv X`, "abc\n"},
		{`export Y=exported; printenv Y`, "exported\n"},
		{`Z=local; printenv Z; echo done`, "done\n"},
		{`Z=local printenv Z`, "local\n"},
		{`A=1 B=2 printenv A B`, "1\n2\n"},
	})
}

func TestCommandSubstitution(t *testing.T) {
	checkAll(t, []testCase{
		{"echo $(echo nested)", "nested\n"},
		{"echo `echo backtick`", "backtick\n"},
		{`echo "value: $(printf 'a   b')"`, "value: a   b\n"},
		{"echo $(printf 'a   b')", "a b\n"},
		{"echo $(echo one; echo two)", "one two\n"},
		{"X=$(echo assigned); echo $X", "assigned\n"},
		{"echo $(echo $(echo deep))", "deep\n"},
		{"echo $(printf 'x\\ny') | wc -l", "1\n"},
		{"echo prefix$(echo mid)suffix", "prefixmidsuffix\n"},
	})
}

func TestPipelines(t *testing.T) {
	checkAll(t, []testCase{
		{"echo hello | tr a-z A-Z", "HELLO\n"},
		{"echo hello | tr a-z A-Z | tr -d L", "HEO\n"},
		{"printf 'b\\na\\nc\\n' | sort | head -n 1", "a\n"},
		{"echo builtin in pipe | cat", "builtin in pipe\n"},
		{"printf 'x\\ny\\n' | wc -l | tr -d ' '", "2\n"},
		{"false | true; echo $?", "0\n"},
		{"true | false; echo $?", "1\n"},
	})
}

func TestListsShortCircuit(t *testing.T) {
	checkAll(t, []testCase{
		{"true && echo yes", "yes\n"},
		{"false && echo no; echo after", "after\n"},
		{"false || echo fallback", "fallback\n"},
		{"true || echo skipped; echo end", "end\n"},
		{"true && false || echo recovered", "recovered\n"},
		{"false && echo a || echo b", "b\n"},
		{"false && echo a; echo $?", "1\n"},
	})
}

func TestRedirections(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "out.txt")
	stdout(t, "echo first > "+file)
	stdout(t, "echo second >> "+file)
	if got := stdout(t, "cat < "+file); got != "first\nsecond\n" {
		t.Fatalf("file content = %q", got)
	}
	stdout(t, "echo replaced > "+file)
	if got := stdout(t, "cat "+file); got != "replaced\n" {
		t.Fatalf("truncate: %q", got)
	}
	errFile := filepath.Join(dir, "err.txt")
	out, _, status, _ := run(t, "cat /definitely/missing/file 2> "+errFile)
	if out != "" || status == 0 {
		t.Fatalf("expected failure with empty stdout, got %q status %d", out, status)
	}
	data, _ := os.ReadFile(errFile)
	if !strings.Contains(string(data), "No such file") {
		t.Fatalf("stderr not captured: %q", data)
	}
	if got := stdout(t, "cat /definitely/missing/file 2>&1 | grep -c 'No such'"); strings.TrimSpace(got) != "1" {
		t.Fatalf("2>&1 into pipe: %q", got)
	}
	quoted := filepath.Join(dir, "with space.txt")
	stdout(t, `echo spaced > "`+quoted+`"`)
	if got := stdout(t, `cat "`+quoted+`"`); got != "spaced\n" {
		t.Fatalf("quoted redirect target: %q", got)
	}
	if _, errOut, status, _ := run(t, "cat < /definitely/missing/file"); status != 1 || !strings.Contains(errOut, "missing") {
		t.Fatalf("missing input file: status %d stderr %q", status, errOut)
	}
}

func TestGlobbingAndTilde(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.log"} {
		os.WriteFile(filepath.Join(dir, name), nil, 0o644)
	}
	got := stdout(t, "echo "+filepath.Join(dir, "*.txt"))
	if !strings.Contains(got, "a.txt") || !strings.Contains(got, "b.txt") || strings.Contains(got, "c.log") {
		t.Fatalf("glob = %q", got)
	}
	if got := stdout(t, "echo '"+filepath.Join(dir, "*.txt")+"'"); strings.Contains(got, "a.txt") {
		t.Fatalf("quoted glob should not expand: %q", got)
	}
	if got := stdout(t, "echo "+filepath.Join(dir, "*.none")); !strings.HasSuffix(got, "*.none\n") {
		t.Fatalf("non matching glob should stay literal: %q", got)
	}
	os.Setenv("HOME", dir)
	if got := stdout(t, "echo ~"); got != dir+"\n" {
		t.Fatalf("tilde = %q", got)
	}
	if got := stdout(t, "echo ~/sub"); got != dir+"/sub\n" {
		t.Fatalf("tilde with path = %q", got)
	}
}

func TestBuiltins(t *testing.T) {
	dir := t.TempDir()
	if got := stdout(t, "cd "+dir+" && pwd"); strings.TrimSpace(got) != dir {
		if resolved, _ := filepath.EvalSymlinks(dir); strings.TrimSpace(got) != resolved {
			t.Fatalf("cd/pwd = %q", got)
		}
	}
	if _, errOut, status, _ := run(t, "cd /definitely/missing"); status != 1 || !strings.Contains(errOut, "cd:") {
		t.Fatalf("cd missing: %d %q", status, errOut)
	}
	if got := stdout(t, "alias greet='echo hello'; greet world"); got != "hello world\n" {
		t.Fatalf("alias = %q", got)
	}
	if got := stdout(t, "alias ll='ls -l'; alias ll"); got != "alias ll='ls -l'\n" {
		t.Fatalf("alias listing = %q", got)
	}
	if got := stdout(t, "alias x='echo x'; unalias x; type x 2>&1"); !strings.Contains(got, "not found") {
		t.Fatalf("unalias = %q", got)
	}
	if got := stdout(t, "type cd echo"); got != "cd is a shell builtin\necho is a shell builtin\n" {
		t.Fatalf("type = %q", got)
	}
	if got := stdout(t, "type cat"); !strings.Contains(got, "cat is /") {
		t.Fatalf("type external = %q", got)
	}
	script := filepath.Join(dir, "rc.sh")
	os.WriteFile(script, []byte("SOURCED=yes\nalias hi='echo hi from rc'\n"), 0o644)
	if got := stdout(t, "source "+script+"; echo $SOURCED; hi"); got != "yes\nhi from rc\n" {
		t.Fatalf("source = %q", got)
	}
	if got := stdout(t, "help | head -c 14"); got != "gosh builtins:" {
		t.Fatalf("help = %q", got)
	}
}

func TestExitAndStatus(t *testing.T) {
	var out strings.Builder
	sh := NewShell(strings.NewReader(""), &out, &out)
	err := sh.Run("echo before; exit 3; echo after")
	if err != ErrExit || sh.exitCode != 3 || out.String() != "before\n" {
		t.Fatalf("exit: err=%v code=%d out=%q", err, sh.exitCode, out.String())
	}
	_, errOut, status, _ := run(t, "definitely-not-a-command-xyz")
	if status != 127 || !strings.Contains(errOut, "command not found") {
		t.Fatalf("missing command: %d %q", status, errOut)
	}
	if _, _, status, _ := run(t, "sh -c 'exit 42'"); status != 42 {
		t.Fatalf("external exit status = %d", status)
	}
	if _, _, _, err := run(t, "echo 'unterminated"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestBackgroundDoesNotBlock(t *testing.T) {
	start := time.Now()
	stdout(t, "sleep 2 &")
	if time.Since(start) > time.Second {
		t.Fatal("background job blocked the shell")
	}
}

func TestScriptFile(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "test.sh")
	os.WriteFile(script, []byte("#!/usr/bin/env gosh\nX=script\necho running $X\n\necho $X | tr a-z A-Z\nexit 7\necho unreachable\n"), 0o644)
	var out strings.Builder
	sh := NewShell(strings.NewReader(""), &out, &out)
	f, _ := os.Open(script)
	defer f.Close()
	code := sh.runScript(newScanner(f))
	if code != 7 || out.String() != "running script\nSCRIPT\n" {
		t.Fatalf("script: code=%d out=%q", code, out.String())
	}
}
