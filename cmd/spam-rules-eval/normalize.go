package main

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"io"
	"regexp"
	"unicode"
	"unicode/utf8"

	"github.com/git-pkgs/spam/internal/textutil"
	"golang.org/x/net/html"
)

const normalizedView = "html-text-v2"

type sourceSpan struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type textView struct {
	Text    []byte
	Sources []sourceSpan
}

type textBuilder struct {
	Views    []textView
	Current  textView
	Newlines int
}

var entityReference = regexp.MustCompile(`&(?:#[xX][0-9a-fA-F]+|#[0-9]+|[a-zA-Z][a-zA-Z0-9]+);`)

type htmlText struct {
	builder textBuilder
	ignored string
	depth   int
}

func normalizeText(data []byte) ([]textView, error) {
	tokenizer := html.NewTokenizer(bytes.NewReader(data))
	parser := htmlText{}
	offset := 0
	for {
		kind := tokenizer.Next()
		start := offset
		offset += len(tokenizer.Raw())
		if kind == html.ErrorToken {
			if tokenizer.Err() != io.EOF {
				return nil, tokenizer.Err()
			}
			parser.builder.flush()
			return parser.builder.Views, nil
		}
		if kind == html.TextToken {
			if parser.ignored == "" {
				parser.builder.text(data[start:offset], start)
			}
			continue
		}
		if kind != html.StartTagToken && kind != html.EndTagToken && kind != html.SelfClosingTagToken {
			continue
		}
		nameBytes, _ := tokenizer.TagName()
		parser.tag(string(nameBytes), kind, sourceSpan{start, offset})
	}
}

func (p *htmlText) tag(name string, kind html.TokenType, span sourceSpan) {
	if p.ignored != "" {
		if name == p.ignored {
			if kind == html.StartTagToken || kind == html.SelfClosingTagToken {
				p.depth++
			}
			if kind == html.EndTagToken {
				p.depth--
				if p.depth == 0 {
					p.ignored = ""
					p.builder.flush()
				}
			}
		}
		return
	}
	switch name {
	case "script", "style", "template":
		p.builder.flush()
		if kind == html.StartTagToken || kind == html.SelfClosingTagToken {
			p.ignored = name
			p.depth = 1
		}
	case "br":
		p.builder.write(' ', span)
	case "a", "abbr", "b", "bdi", "bdo", "cite", "code", "del", "em", "font", "i", "ins", "kbd", "mark", "q", "ruby", "s", "samp", "small", "span", "strong", "sub", "sup", "time", "u", "var", "wbr":
		// Inline tags may split a word without introducing a space.
	default:
		p.builder.flush()
	}
}

func (b *textBuilder) text(data []byte, offset int) {
	cursor := 0
	for _, entity := range entityReference.FindAllIndex(data, -1) {
		b.literal(data[cursor:entity[0]], offset+cursor)
		raw := string(data[entity[0]:entity[1]])
		decoded := stdhtml.UnescapeString(raw)
		if decoded == raw {
			b.literal(data[entity[0]:entity[1]], offset+entity[0])
		} else {
			span := sourceSpan{offset + entity[0], offset + entity[1]}
			for _, r := range decoded {
				b.write(r, span)
			}
		}
		cursor = entity[1]
	}
	b.literal(data[cursor:], offset+cursor)
}

func (b *textBuilder) literal(data []byte, offset int) {
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		b.write(r, sourceSpan{offset + i, offset + i + size})
		i += size
	}
}

func (b *textBuilder) write(r rune, span sourceSpan) {
	const paragraphNewlines = 2
	if unicode.IsSpace(r) {
		if r == '\n' {
			b.Newlines++
		}
		if b.Newlines >= paragraphNewlines {
			b.flush()
			return
		}
		if len(b.Current.Text) == 0 {
			return
		}
		last := len(b.Current.Text) - 1
		if b.Current.Text[last] == ' ' {
			b.Current.Sources[last].End = span.End
			return
		}
		b.Current.Text = append(b.Current.Text, ' ')
		b.Current.Sources = append(b.Current.Sources, span)
		return
	}
	b.Newlines = 0
	before := len(b.Current.Text)
	b.Current.Text = utf8.AppendRune(b.Current.Text, r)
	for range len(b.Current.Text) - before {
		b.Current.Sources = append(b.Current.Sources, span)
	}
}

func (b *textBuilder) flush() {
	if last := len(b.Current.Text) - 1; last >= 0 && b.Current.Text[last] == ' ' {
		b.Current.Text = b.Current.Text[:last]
		b.Current.Sources = b.Current.Sources[:last]
	}
	if len(b.Current.Text) > 0 {
		b.Views = append(b.Views, b.Current)
	}
	b.Current = textView{}
	b.Newlines = 0
}

func mapEvidence(h hit, view textView, original []byte) (hit, error) {
	if h.Start < 0 || h.End > len(view.Sources) || h.Start >= h.End {
		return hit{}, fmt.Errorf("invalid normalized evidence")
	}
	spans := []sourceSpan{}
	for _, span := range view.Sources[h.Start:h.End] {
		if span.Start < 0 || span.End > len(original) || span.Start >= span.End {
			return hit{}, fmt.Errorf("invalid source mapping")
		}
		last := len(spans) - 1
		if last >= 0 && span.Start <= spans[last].End {
			if span.End > spans[last].End {
				spans[last].End = span.End
			}
		} else {
			spans = append(spans, span)
		}
	}
	h.View = normalizedView
	h.MatchedText = h.Excerpt
	h.OriginalSpans = spans
	h.Start = spans[0].Start
	h.End = spans[len(spans)-1].End
	h.Excerpt = textutil.Excerpt(original[h.Start:h.End])
	return h, nil
}

func (r rule) inScope(scope string) bool {
	switch scope {
	case normalizedScope:
		return r.Scope == documentScope && (r.UpstreamInput == "body" || r.UpstreamInput == "sa_body")
	case rawScope:
		return r.Scope == documentScope && r.UpstreamInput == "sa_raw_body"
	default:
		return r.Scope == scope
	}
}

func (e *engine) normalizedMatches(data []byte, path string) ([]hit, int, int, error) {
	views, err := normalizeText(data)
	if err != nil {
		return nil, 0, 0, err
	}
	found, err := e.match(data, rawScope, path, 0)
	if err != nil {
		return nil, 0, 0, err
	}
	seen := map[string]bool{}
	normalizedBytes := 0
	for _, view := range views {
		normalizedBytes += len(view.Text)
		matches, err := e.match(view.Text, normalizedScope, path, 0)
		if err != nil {
			return nil, 0, 0, err
		}
		for _, h := range matches {
			if seen[h.Rule] {
				continue
			}
			mapped, err := mapEvidence(h, view, data)
			if err != nil {
				return nil, 0, 0, err
			}
			found = append(found, mapped)
			seen[h.Rule] = true
		}
	}
	return found, len(views), normalizedBytes, nil
}

func viewMode(view string) string {
	if view == normalizedMode {
		return "UTF-8 HTML text and normalized whitespace within paragraphs; raw markup rules and lexical URLs unchanged; boolean candidates only; no upstream scores"
	}
	return "UTF-8 raw documents and lexical http/https/file URLs; boolean candidates only; no upstream preprocessing or scores"
}

func supportedInput(r rule) bool {
	return r.Scope == urlScope || r.UpstreamInput == "body" || r.UpstreamInput == "sa_body" || r.UpstreamInput == "sa_raw_body"
}
