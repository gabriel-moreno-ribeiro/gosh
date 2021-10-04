package main

import (
	"fmt"
	"strings"
)

// TokenKind classifies lexer output.
type TokenKind int

const (
	// TokWord is a word, still containing its quotes and $expansions.
	TokWord TokenKind = iota
	// TokPipe is |
	TokPipe
	// TokAnd is &&
	TokAnd
	// TokOr is ||
	TokOr
	// TokSemi is ;
	TokSemi
	// TokAmp is & (run in background)
	TokAmp
	// TokRedirect is one of < > >> 2> 2>> 2>&1; Text holds the operator.
	TokRedirect
	// TokNewline separates commands like ;
	TokNewline
	// TokEOF ends the token stream.
	TokEOF
)

// Token is a lexed piece of input.
type Token struct {
	Kind TokenKind
	Text string
}

// Lex splits a command line into tokens. Words keep their quotes and
// expansion syntax verbatim; they are interpreted later by expandWord so
// that quoting rules and variable expansion are handled in one place.
func Lex(input string) ([]Token, error) {
	var tokens []Token
	i := 0
	n := len(input)

	for i < n {
		c := input[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '\n':
			tokens = append(tokens, Token{TokNewline, "\n"})
			i++
		case c == '#':
			// comment until end of line
			for i < n && input[i] != '\n' {
				i++
			}
		case c == '|':
			if i+1 < n && input[i+1] == '|' {
				tokens = append(tokens, Token{TokOr, "||"})
				i += 2
			} else {
				tokens = append(tokens, Token{TokPipe, "|"})
				i++
			}
		case c == '&':
			if i+1 < n && input[i+1] == '&' {
				tokens = append(tokens, Token{TokAnd, "&&"})
				i += 2
			} else {
				tokens = append(tokens, Token{TokAmp, "&"})
				i++
			}
		case c == ';':
			tokens = append(tokens, Token{TokSemi, ";"})
			i++
		case c == '<':
			tokens = append(tokens, Token{TokRedirect, "<"})
			i++
		case c == '>':
			if i+1 < n && input[i+1] == '>' {
				tokens = append(tokens, Token{TokRedirect, ">>"})
				i += 2
			} else {
				tokens = append(tokens, Token{TokRedirect, ">"})
				i++
			}
		case c == '2' && i+1 < n && input[i+1] == '>' && (i == 0 || isSeparator(input[i-1])):
			if strings.HasPrefix(input[i:], "2>&1") {
				tokens = append(tokens, Token{TokRedirect, "2>&1"})
				i += 4
			} else if strings.HasPrefix(input[i:], "2>>") {
				tokens = append(tokens, Token{TokRedirect, "2>>"})
				i += 3
			} else {
				tokens = append(tokens, Token{TokRedirect, "2>"})
				i += 2
			}
		default:
			word, next, err := lexWord(input, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, Token{TokWord, word})
			i = next
		}
	}
	tokens = append(tokens, Token{TokEOF, ""})
	return tokens, nil
}

func isSeparator(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == ';' || c == '|' || c == '&'
}

func isWordEnd(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ';' || c == '|' || c == '&' || c == '<' || c == '>'
}

// lexWord scans one word starting at i, honouring quotes, escapes and
// $( ... ) / backtick nesting so that spaces inside them do not end the word.
func lexWord(input string, i int) (string, int, error) {
	n := len(input)
	start := i
	for i < n {
		c := input[i]
		switch {
		case c == '\\':
			i += 2
		case c == '\'':
			end := strings.IndexByte(input[i+1:], '\'')
			if end < 0 {
				return "", 0, fmt.Errorf("unterminated single quote")
			}
			i += end + 2
		case c == '"':
			j := i + 1
			for j < n && input[j] != '"' {
				if input[j] == '\\' {
					j++
				}
				j++
			}
			if j >= n {
				return "", 0, fmt.Errorf("unterminated double quote")
			}
			i = j + 1
		case c == '`':
			end := strings.IndexByte(input[i+1:], '`')
			if end < 0 {
				return "", 0, fmt.Errorf("unterminated backtick")
			}
			i += end + 2
		case c == '$' && i+1 < n && input[i+1] == '(':
			depth := 0
			j := i + 1
			for j < n {
				if input[j] == '(' {
					depth++
				} else if input[j] == ')' {
					depth--
					if depth == 0 {
						break
					}
				}
				j++
			}
			if j >= n {
				return "", 0, fmt.Errorf("unterminated command substitution")
			}
			i = j + 1
		case isWordEnd(c):
			return input[start:i], i, nil
		default:
			i++
		}
	}
	if i > n {
		i = n
	}
	return input[start:i], i, nil
}
