# gosh

A POSIX-style shell written from scratch in Go. It has a lexer, a parser and
an interpreter for the parts of the shell language people use every day:
pipelines, `&&`/`||`/`;` lists, background jobs, redirections, quoting,
variables, command substitution, globbing, aliases and a handful of builtins.
No dependencies outside the Go standard library.

```sh
go build -o gosh .
./gosh                       # interactive
./gosh script.sh             # run a script
./gosh -c 'ls | wc -l'       # run one command line
```

## What works

```sh
cat *.go | grep -c func                       # pipes and globbing
make && ./app || echo "build failed"          # && || short-circuit lists
sort < in.txt > out.txt 2> errors.log         # < > >> 2> 2>> 2>&1
NAME="world"; echo "hello $NAME ${NAME}!"     # variables, quoting
files=$(ls | wc -l); echo "$files files"      # command substitution, $(..) and `..`
GOOS=linux go build                           # per-command environment
alias ll='ls -la'; ll ~/projects              # aliases, tilde expansion
sleep 10 &                                    # background jobs
false; echo $?                                # exit status
source ~/.goshrc                              # run a file in the current shell
```

Builtins: `cd` `pwd` `echo` `exit` `export` `unset` `env` `set` `alias`
`unalias` `history` `type` `true` `false` `source` `.` `help`.

## How it works

1. **Lexer** (`lexer.go`) splits the line into words and operators. A word
   keeps its quotes and `$...` syntax verbatim, but the lexer understands
   quoting and `$( ... )` nesting so that spaces inside them do not end the
   word.
2. **Parser** (`parser.go`) builds a small AST: a list of pipelines joined by
   `&&`, `||`, `;` or `&`; each pipeline is a list of commands; each command
   has words and redirections.
3. **Expansion** (`expand.go`) runs at execution time, one word at a time:
   tilde, quote removal, `$VAR` / `${VAR}` / `$?` / `$$`, command
   substitution (executed in a child shell writing to a buffer), field
   splitting of unquoted expansions, then globbing with `filepath.Glob`.
4. **Execution** (`shell.go`, `builtins.go`): builtins receive the command's
   stdin/stdout/stderr as plain `io` values, so they work inside pipelines and
   under redirections exactly like external programs. Pipelines connect
   commands with `os.Pipe` and run each stage in a goroutine; external
   programs use `os/exec`. `&&`/`||` are evaluated left to right against the
   status of the previous pipeline.

## Tests

```sh
go test ./...
```

The tests exercise the lexer and parser directly, then run real command lines
through the shell (using `cat`, `tr`, `sort`, `printenv`, etc.) and compare the
captured output.

## License

MIT
