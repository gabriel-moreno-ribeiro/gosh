# gosh

> 🇺🇸 [English version below](#english)

Um shell estilo POSIX em Go. Lexer, parser e interpretador pras partes da linguagem do shell que a gente usa todo dia: pipes, `&&`/`||`/`;`, jobs em background, redirecionamentos, aspas, variáveis, command substitution, globbing, aliases e uns builtins. Nada fora da biblioteca padrão do Go.

Aprendi Go fazendo isso e foi uma boa escolha: goroutines + `os.Pipe` deixam um pipeline de N comandos parecer coisa de brinquedo.

```sh
go build -o gosh .
./gosh                       # interativo
./gosh script.sh
./gosh -c 'ls | wc -l'
```

```sh
cat *.go | grep -c func                       # pipes e globbing
make && ./app || echo "build failed"          # listas com curto-circuito
sort < in.txt > out.txt 2> errors.log         # < > >> 2> 2>> 2>&1
NAME="world"; echo "hello $NAME ${NAME}!"     # variáveis e aspas
files=$(ls | wc -l); echo "$files files"      # $(..) e `..`
GOOS=linux go build                           # ambiente por comando
alias ll='ls -la'; ll ~/projects              # alias e til
sleep 10 &                                    # background
false; echo $?                                # status de saída
source ~/.goshrc
```

Builtins: `cd pwd echo exit export unset env set alias unalias history type true false source . help`.

## Por dentro

1. O **lexer** (`lexer.go`) separa palavras e operadores. Uma palavra guarda as aspas e o `$...` como estão, mas o lexer entende aspas e `$( ... )` aninhado o suficiente pra um espaço lá dentro não terminar a palavra.
2. O **parser** (`parser.go`) monta uma AST pequena: lista de pipelines ligados por `&&`, `||`, `;` ou `&`; cada pipeline é uma lista de comandos; cada comando tem palavras e redirecionamentos.
3. A **expansão** (`expand.go`) acontece na hora de executar, palavra por palavra e nessa ordem: til, remoção de aspas, `$VAR`/`${VAR}`/`$?`/`$$`, command substitution (roda num shell filho escrevendo num buffer), field splitting do que não estava entre aspas, e por fim glob.
4. A **execução** (`shell.go`, `builtins.go`): builtins recebem stdin/stdout/stderr como valores `io`, então funcionam dentro de pipes e com redirecionamento igual a programa externo. Pipelines ligam os comandos com `os.Pipe` e rodam cada estágio numa goroutine.

A coisa mais sutil que eu errei na primeira versão: `a && b || c` é avaliado da esquerda pra direita olhando o status do pipeline anterior, não é uma árvore de precedência. Tinha teste falhando por isso e demorei pra entender que o *bash* que estava certo.

Testes: `go test ./...` (lexer, parser e linhas de comando reais passando por `cat`, `tr`, `sort`, `printenv`).

---

## English

A POSIX-style shell in Go. Lexer, parser and interpreter for the parts of the shell language we use every day: pipes, `&&`/`||`/`;`, background jobs, redirections, quoting, variables, command substitution, globbing, aliases and a few builtins. Nothing outside Go's standard library.

I learned Go doing this and it was a good choice: goroutines + `os.Pipe` make a pipeline of N commands look like a toy.

```sh
go build -o gosh .
./gosh                       # interactive
./gosh script.sh
./gosh -c 'ls | wc -l'
```

```sh
cat *.go | grep -c func                       # pipes and globbing
make && ./app || echo "build failed"          # short-circuit lists
sort < in.txt > out.txt 2> errors.log         # < > >> 2> 2>> 2>&1
NAME="world"; echo "hello $NAME ${NAME}!"     # variables and quotes
files=$(ls | wc -l); echo "$files files"      # $(..) and `..`
GOOS=linux go build                           # per-command environment
alias ll='ls -la'; ll ~/projects              # alias and tilde
sleep 10 &                                    # background
false; echo $?                                # exit status
source ~/.goshrc
```

Builtins: `cd pwd echo exit export unset env set alias unalias history type true false source . help`.

## Inside

1. The **lexer** (`lexer.go`) splits words and operators. A word keeps the quotes and the `$...` as they are, but the lexer understands quotes and nested `$( ... )` well enough that a space in there doesn't end the word.
2. The **parser** (`parser.go`) builds a small AST: a list of pipelines joined by `&&`, `||`, `;` or `&`; each pipeline is a list of commands; each command has words and redirections.
3. **Expansion** (`expand.go`) happens at execution time, word by word and in this order: tilde, quote removal, `$VAR`/`${VAR}`/`$?`/`$$`, command substitution (runs in a child shell writing to a buffer), field splitting of whatever wasn't quoted, and finally glob.
4. **Execution** (`shell.go`, `builtins.go`): builtins receive stdin/stdout/stderr as `io` values, so they work inside pipes and with redirection just like an external program. Pipelines connect the commands with `os.Pipe` and run each stage in a goroutine.

The most subtle thing I got wrong in the first version: `a && b || c` is evaluated left to right looking at the status of the previous pipeline, it's not a precedence tree. I had a failing test because of this and it took me a while to understand that *bash* was the one that was right.

Tests: `go test ./...` (lexer, parser and real command lines going through `cat`, `tr`, `sort`, `printenv`).

MIT.
