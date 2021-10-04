package main

import "fmt"

// Redirect is one file redirection attached to a command.
type Redirect struct {
	Op     string // < > >> 2> 2>> 2>&1
	Target string // file name (raw word), empty for 2>&1
}

// Command is a simple command: words plus redirections.
type Command struct {
	Words     []string // raw words, expanded at execution time
	Redirects []Redirect
}

// Pipeline is one or more commands connected by pipes.
type Pipeline struct {
	Commands   []*Command
	Background bool
}

// ListItem is a pipeline plus the operator that joins it to the next one.
type ListItem struct {
	Pipeline *Pipeline
	Op       string // "&&", "||", ";" or "" for the last item
}

// Parse turns tokens into a list of pipelines.
//
//	list     := pipeline (("&&" | "||" | ";" | "&" | newline) pipeline)*
//	pipeline := command ("|" command)*
//	command  := (word | redirect word)+
func Parse(tokens []Token) ([]ListItem, error) {
	p := &parser{tokens: tokens}
	var items []ListItem
	for {
		p.skipNewlines()
		if p.peek().Kind == TokEOF {
			break
		}
		pipe, err := p.parsePipeline()
		if err != nil {
			return nil, err
		}
		item := ListItem{Pipeline: pipe}
		switch p.peek().Kind {
		case TokAnd:
			item.Op = "&&"
			p.next()
		case TokOr:
			item.Op = "||"
			p.next()
		case TokSemi, TokNewline:
			item.Op = ";"
			p.next()
		case TokAmp:
			pipe.Background = true
			item.Op = ";"
			p.next()
		case TokEOF:
		default:
			return nil, fmt.Errorf("syntax error near %q", p.peek().Text)
		}
		items = append(items, item)
	}
	return items, nil
}

type parser struct {
	tokens []Token
	pos    int
}

func (p *parser) peek() Token { return p.tokens[p.pos] }

func (p *parser) next() Token {
	t := p.tokens[p.pos]
	if t.Kind != TokEOF {
		p.pos++
	}
	return t
}

func (p *parser) skipNewlines() {
	for p.peek().Kind == TokNewline {
		p.next()
	}
}

func (p *parser) parsePipeline() (*Pipeline, error) {
	pipe := &Pipeline{}
	for {
		cmd, err := p.parseCommand()
		if err != nil {
			return nil, err
		}
		pipe.Commands = append(pipe.Commands, cmd)
		if p.peek().Kind != TokPipe {
			return pipe, nil
		}
		p.next()
		p.skipNewlines()
	}
}

func (p *parser) parseCommand() (*Command, error) {
	cmd := &Command{}
	for {
		t := p.peek()
		switch t.Kind {
		case TokWord:
			cmd.Words = append(cmd.Words, t.Text)
			p.next()
		case TokRedirect:
			p.next()
			r := Redirect{Op: t.Text}
			if t.Text != "2>&1" {
				target := p.next()
				if target.Kind != TokWord {
					return nil, fmt.Errorf("syntax error: expected file after %s", t.Text)
				}
				r.Target = target.Text
			}
			cmd.Redirects = append(cmd.Redirects, r)
		default:
			if len(cmd.Words) == 0 && len(cmd.Redirects) == 0 {
				return nil, fmt.Errorf("syntax error near %q", t.Text)
			}
			return cmd, nil
		}
	}
}
