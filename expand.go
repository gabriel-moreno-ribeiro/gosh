package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// expandWord applies tilde expansion, quote removal, variable expansion,
// command substitution, field splitting and globbing to one raw word and
// returns the resulting fields (usually exactly one).
func (sh *Shell) expandWord(raw string) ([]string, error) {
	var fields []string
	var out strings.Builder
	pending := false // out holds a field in progress (possibly empty but quoted)
	quoted := false  // any quoting seen: disables globbing on the result

	flush := func() {
		fields = append(fields, out.String())
		out.Reset()
		pending = false
	}

	// tilde expansion only at the very start of an unquoted word
	if strings.HasPrefix(raw, "~") && (len(raw) == 1 || raw[1] == '/') {
		out.WriteString(sh.home())
		raw = raw[1:]
		pending = true
	}

	i := 0
	n := len(raw)
	for i < n {
		c := raw[i]
		switch c {
		case '\\':
			if i+1 < n {
				out.WriteByte(raw[i+1])
			}
			quoted, pending = true, true
			i += 2
		case '\'':
			end := strings.IndexByte(raw[i+1:], '\'')
			if end < 0 {
				return nil, fmt.Errorf("unterminated single quote")
			}
			out.WriteString(raw[i+1 : i+1+end])
			quoted, pending = true, true
			i += end + 2
		case '"':
			j := i + 1
			for j < n && raw[j] != '"' {
				switch {
				case raw[j] == '\\' && j+1 < n && strings.IndexByte("\"\\$`", raw[j+1]) >= 0:
					out.WriteByte(raw[j+1])
					j += 2
				case raw[j] == '$' || raw[j] == '`':
					val, next, err := sh.expandDollar(raw, j)
					if err != nil {
						return nil, err
					}
					out.WriteString(val)
					j = next
				default:
					out.WriteByte(raw[j])
					j++
				}
			}
			if j >= n {
				return nil, fmt.Errorf("unterminated double quote")
			}
			quoted, pending = true, true
			i = j + 1
		case '$', '`':
			val, next, err := sh.expandDollar(raw, i)
			if err != nil {
				return nil, err
			}
			i = next
			// unquoted expansion results are split into fields on whitespace
			if val == "" {
				continue
			}
			leadingSpace := unicode.IsSpace(rune(val[0]))
			trailingSpace := unicode.IsSpace(rune(val[len(val)-1]))
			parts := strings.Fields(val)
			if leadingSpace && pending {
				flush()
			}
			for k, part := range parts {
				if k > 0 {
					flush()
				}
				out.WriteString(part)
				pending = true
			}
			if trailingSpace && pending {
				flush()
			}
		default:
			out.WriteByte(c)
			pending = true
			i++
		}
	}
	if pending {
		flush()
	}

	if quoted {
		return fields, nil
	}
	// globbing on unquoted words containing wildcards
	var result []string
	for _, f := range fields {
		if strings.ContainsAny(f, "*?[") {
			matches, err := filepath.Glob(f)
			if err == nil && len(matches) > 0 {
				result = append(result, matches...)
				continue
			}
		}
		result = append(result, f)
	}
	return result, nil
}

// expandDollar handles $VAR, ${VAR}, $?, $$, $(cmd) and `cmd` starting at
// raw[i]. Returns the value and the index just after the expansion.
func (sh *Shell) expandDollar(raw string, i int) (string, int, error) {
	n := len(raw)
	if raw[i] == '`' {
		end := strings.IndexByte(raw[i+1:], '`')
		if end < 0 {
			return "", 0, fmt.Errorf("unterminated backtick")
		}
		val, err := sh.commandSubstitution(raw[i+1 : i+1+end])
		return val, i + end + 2, err
	}
	if i+1 >= n {
		return "$", i + 1, nil
	}
	c := raw[i+1]
	switch {
	case c == '(':
		depth := 0
		j := i + 1
		for j < n {
			if raw[j] == '(' {
				depth++
			} else if raw[j] == ')' {
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
		val, err := sh.commandSubstitution(raw[i+2 : j])
		return val, j + 1, err
	case c == '{':
		end := strings.IndexByte(raw[i:], '}')
		if end < 0 {
			return "", 0, fmt.Errorf("unterminated ${")
		}
		return sh.getVar(raw[i+2 : i+end]), i + end + 1, nil
	case c == '?':
		return fmt.Sprint(sh.lastStatus), i + 2, nil
	case c == '$':
		return fmt.Sprint(os.Getpid()), i + 2, nil
	case isVarStart(c):
		j := i + 1
		for j < n && isVarChar(raw[j]) {
			j++
		}
		return sh.getVar(raw[i+1 : j]), j, nil
	default:
		return "$", i + 1, nil
	}
}

func isVarStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isVarChar(c byte) bool {
	return isVarStart(c) || (c >= '0' && c <= '9')
}

// commandSubstitution runs a command line in a child shell and returns its
// standard output with trailing newlines removed.
func (sh *Shell) commandSubstitution(cmdline string) (string, error) {
	var buf strings.Builder
	sub := sh.child(&buf)
	if err := sub.Run(cmdline); err != nil && err != ErrExit {
		return "", err
	}
	sh.lastStatus = sub.lastStatus
	return strings.TrimRight(buf.String(), "\n"), nil
}

func (sh *Shell) home() string {
	if h := sh.getVar("HOME"); h != "" {
		return h
	}
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "/"
}
