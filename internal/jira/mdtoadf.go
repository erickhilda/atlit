package jira

import (
	"regexp"
	"strings"
)

// MarkdownToADF converts a markdown string to an ADFDoc.
// Supports headings, paragraphs, bullet/ordered lists, fenced code blocks,
// blockquotes, and inline marks (bold, italic, code, strikethrough, links).
func MarkdownToADF(md string) *ADFDoc {
	p := &mdParser{lines: strings.Split(md, "\n")}
	nodes := p.parseBlocks()
	return &ADFDoc{Type: "doc", Version: 1, Content: nodes}
}

type mdParser struct {
	lines []string
	pos   int
}

func (p *mdParser) peek() (string, bool) {
	if p.pos >= len(p.lines) {
		return "", false
	}
	return p.lines[p.pos], true
}

func (p *mdParser) consume() string {
	line := p.lines[p.pos]
	p.pos++
	return line
}

func (p *mdParser) parseBlocks() []ADFNode {
	var nodes []ADFNode
	for p.pos < len(p.lines) {
		line, _ := p.peek()
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "":
			p.consume()

		case strings.HasPrefix(trimmed, "```"):
			if n := p.parseCodeBlock(); n != nil {
				nodes = append(nodes, *n)
			}

		case headingLevel(trimmed) > 0:
			nodes = append(nodes, p.parseHeading())

		case strings.HasPrefix(trimmed, "> ") || trimmed == ">":
			nodes = append(nodes, p.parseBlockquote())

		case isUnorderedItem(trimmed):
			nodes = append(nodes, p.parseList(false, 0))

		case isOrderedItem(trimmed):
			nodes = append(nodes, p.parseList(true, 0))

		default:
			nodes = append(nodes, p.parseParagraph())
		}
	}
	return nodes
}

func (p *mdParser) parseHeading() ADFNode {
	line := strings.TrimSpace(p.consume())
	// Count leading '#' chars to determine heading level.
	count := 0
	for _, ch := range line {
		if ch == '#' {
			count++
		} else {
			break
		}
	}
	text := strings.TrimSpace(line[count:])
	return ADFNode{
		Type:    "heading",
		Attrs:   map[string]any{"level": count},
		Content: parseInline(text),
	}
}

func (p *mdParser) parseParagraph() ADFNode {
	var parts []string
	for {
		line, ok := p.peek()
		if !ok {
			break
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		if headingLevel(trimmed) > 0 || strings.HasPrefix(trimmed, "```") ||
			strings.HasPrefix(trimmed, "> ") || trimmed == ">" ||
			isUnorderedItem(trimmed) || isOrderedItem(trimmed) {
			break
		}
		parts = append(parts, strings.TrimSpace(p.consume()))
	}
	text := strings.Join(parts, " ")
	return ADFNode{
		Type:    "paragraph",
		Content: parseInline(text),
	}
}

func (p *mdParser) parseCodeBlock() *ADFNode {
	line := strings.TrimSpace(p.consume()) // opening ```
	lang := strings.TrimPrefix(line, "```")
	lang = strings.TrimSpace(lang)

	var codeLines []string
	for {
		line, ok := p.peek()
		if !ok {
			break
		}
		if strings.TrimSpace(line) == "```" {
			p.consume()
			break
		}
		codeLines = append(codeLines, p.consume())
	}
	code := strings.Join(codeLines, "\n")
	node := ADFNode{
		Type:    "codeBlock",
		Content: []ADFNode{{Type: "text", Text: code}},
	}
	if lang != "" {
		node.Attrs = map[string]any{"language": lang}
	}
	return &node
}

func (p *mdParser) parseBlockquote() ADFNode {
	var innerLines []string
	for {
		line, ok := p.peek()
		if !ok {
			break
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "> ") {
			innerLines = append(innerLines, strings.TrimPrefix(trimmed, "> "))
			p.consume()
		} else if trimmed == ">" {
			innerLines = append(innerLines, "")
			p.consume()
		} else {
			break
		}
	}
	inner := strings.Join(innerLines, "\n")
	innerParser := &mdParser{lines: strings.Split(inner, "\n")}
	innerNodes := innerParser.parseBlocks()
	return ADFNode{Type: "blockquote", Content: innerNodes}
}

func (p *mdParser) parseList(ordered bool, depth int) ADFNode {
	listType := "bulletList"
	if ordered {
		listType = "orderedList"
	}
	var items []ADFNode
	for {
		line, ok := p.peek()
		if !ok {
			break
		}
		trimmed := strings.TrimSpace(line)
		indent := leadingSpaces(line)

		// Stop if line is blank (end of list) or a non-list line at this depth.
		if trimmed == "" {
			p.consume()
			// Peek ahead; if next line continues the list, keep going.
			if next, ok2 := p.peek(); ok2 {
				if t := strings.TrimSpace(next); isUnorderedItem(t) || isOrderedItem(t) {
					continue
				}
			}
			break
		}

		// Nested list (more indented).
		if indent > depth*2 {
			if len(items) == 0 {
				break
			}
			// Attach nested list to last item.
			subOrdered := isOrderedItem(trimmed)
			sub := p.parseList(subOrdered, depth+1)
			last := &items[len(items)-1]
			last.Content = append(last.Content, sub)
			continue
		}

		if !isUnorderedItem(trimmed) && !isOrderedItem(trimmed) {
			break
		}

		p.consume()
		text := listItemText(trimmed)
		item := ADFNode{
			Type: "listItem",
			Content: []ADFNode{
				{Type: "paragraph", Content: parseInline(text)},
			},
		}
		items = append(items, item)
	}
	return ADFNode{Type: listType, Content: items}
}

// --- Inline parsing ---

var (
	reBold      = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic    = regexp.MustCompile(`\*(.+?)\*`)
	reCode      = regexp.MustCompile("`(.+?)`")
	reStrike    = regexp.MustCompile(`~~(.+?)~~`)
	reLink      = regexp.MustCompile(`\[(.+?)\]\((.+?)\)`)
)

// parseInline converts a markdown inline string into ADF inline nodes.
// Strategy: find the leftmost match of any pattern, emit text before it,
// emit the match, then recurse on the remainder.
func parseInline(text string) []ADFNode {
	if text == "" {
		return nil
	}

	type match struct {
		start, end int
		node       ADFNode
	}

	best := match{start: len(text) + 1}

	tryMatch := func(re *regexp.Regexp, build func([]string) ADFNode) {
		loc := re.FindStringSubmatchIndex(text)
		if loc != nil && loc[0] < best.start {
			groups := make([]string, len(loc)/2)
			for i := range groups {
				groups[i] = text[loc[2*i]:loc[2*i+1]]
			}
			best = match{start: loc[0], end: loc[1], node: build(groups)}
		}
	}

	tryMatch(reBold, func(g []string) ADFNode {
		return ADFNode{Type: "text", Text: g[1], Marks: []ADFMark{{Type: "strong"}}}
	})
	tryMatch(reItalic, func(g []string) ADFNode {
		return ADFNode{Type: "text", Text: g[1], Marks: []ADFMark{{Type: "em"}}}
	})
	tryMatch(reCode, func(g []string) ADFNode {
		return ADFNode{Type: "text", Text: g[1], Marks: []ADFMark{{Type: "code"}}}
	})
	tryMatch(reStrike, func(g []string) ADFNode {
		return ADFNode{Type: "text", Text: g[1], Marks: []ADFMark{{Type: "strike"}}}
	})
	tryMatch(reLink, func(g []string) ADFNode {
		return ADFNode{
			Type:  "text",
			Text:  g[1],
			Marks: []ADFMark{{Type: "link", Attrs: map[string]any{"href": g[2]}}},
		}
	})

	if best.start > len(text) {
		// No markup found.
		return []ADFNode{{Type: "text", Text: text}}
	}

	var nodes []ADFNode
	if best.start > 0 {
		nodes = append(nodes, ADFNode{Type: "text", Text: text[:best.start]})
	}
	nodes = append(nodes, best.node)
	nodes = append(nodes, parseInline(text[best.end:])...)
	return nodes
}

// --- Helpers ---

func headingLevel(line string) int {
	if !strings.HasPrefix(line, "#") {
		return 0
	}
	count := 0
	for _, ch := range line {
		if ch == '#' {
			count++
		} else {
			break
		}
	}
	// Must be followed by a space to be a heading.
	if count < len(line) && line[count] == ' ' {
		return count
	}
	return 0
}

func isUnorderedItem(line string) bool {
	return strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ")
}

func isOrderedItem(line string) bool {
	re := regexp.MustCompile(`^\d+\. `)
	return re.MatchString(line)
}

func listItemText(line string) string {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return strings.TrimSpace(line[2:])
	}
	re := regexp.MustCompile(`^\d+\. `)
	return strings.TrimSpace(re.ReplaceAllString(line, ""))
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}
