package jira

import (
	"fmt"
	"strings"
)

// RenderADF converts an Atlassian Document Format document to markdown.
func RenderADF(doc *ADFDoc) string {
	if doc == nil || len(doc.Content) == 0 {
		return ""
	}
	c := &converter{}
	c.renderNodes(doc.Content, 0)
	return strings.TrimRight(c.buf.String(), "\n")
}

type converter struct {
	buf strings.Builder
}

func (c *converter) renderNodes(nodes []ADFNode, depth int) {
	for i, node := range nodes {
		c.renderNode(node, depth, i)
	}
}

func (c *converter) renderNode(node ADFNode, depth int, index int) {
	switch node.Type {
	case "paragraph":
		c.renderInlineChildren(node.Content, depth)
		c.buf.WriteString("\n\n")
	case "heading":
		level := attrInt(node.Attrs, "level", 1)
		c.buf.WriteString(strings.Repeat("#", level))
		c.buf.WriteString(" ")
		c.renderInlineChildren(node.Content, depth)
		c.buf.WriteString("\n\n")
	case "bulletList":
		c.renderList(node.Content, depth, false)
	case "orderedList":
		c.renderList(node.Content, depth, true)
	case "listItem":
		c.renderListItem(node, depth)
	case "codeBlock":
		lang := attrStr(node.Attrs, "language")
		c.buf.WriteString("```")
		c.buf.WriteString(lang)
		c.buf.WriteString("\n")
		c.renderPlainChildren(node.Content)
		c.buf.WriteString("\n```\n\n")
	case "blockquote":
		content := c.captureNodes(node.Content, depth)
		for _, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
			c.buf.WriteString("> ")
			c.buf.WriteString(line)
			c.buf.WriteString("\n")
		}
		c.buf.WriteString("\n")
	case "table":
		c.renderTable(node.Content)
	case "panel":
		panelType := attrStr(node.Attrs, "panelType")
		label := panelLabel(panelType)
		content := c.captureNodes(node.Content, depth)
		content = strings.TrimRight(content, "\n")
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			c.buf.WriteString("> ")
			if i == 0 && label != "" {
				c.buf.WriteString(label)
				c.buf.WriteString(" ")
			}
			c.buf.WriteString(line)
			c.buf.WriteString("\n")
		}
		c.buf.WriteString("\n")
	case "rule":
		c.buf.WriteString("---\n\n")
	case "mediaSingle", "mediaGroup":
		// Wrappers around one or more media children; each child renders
		// itself (and its own trailing blank line) via the "media" case.
		c.renderNodes(node.Content, depth)
	case "media":
		c.buf.WriteString(mediaMarkdown(node))
		c.buf.WriteString("\n\n")
	default:
		// Unknown block-level node: try to render children.
		if len(node.Content) > 0 {
			c.renderNodes(node.Content, depth)
		}
	}
}

func (c *converter) renderInlineChildren(nodes []ADFNode, depth int) {
	for _, node := range nodes {
		c.renderInline(node, depth)
	}
}

func (c *converter) renderInline(node ADFNode, depth int) {
	switch node.Type {
	case "text":
		text := node.Text
		text = applyMarks(text, node.Marks)
		c.buf.WriteString(text)
	case "mention":
		name := attrStr(node.Attrs, "text")
		if name == "" {
			name = node.Text
		}
		if !strings.HasPrefix(name, "@") {
			name = "@" + name
		}
		c.buf.WriteString(name)
	case "emoji":
		shortName := attrStr(node.Attrs, "shortName")
		if shortName != "" {
			c.buf.WriteString(shortName)
		}
	case "media", "mediaInline":
		c.buf.WriteString(mediaMarkdown(node))
	case "hardBreak":
		c.buf.WriteString("  \n")
	case "inlineCard":
		url := attrStr(node.Attrs, "url")
		if url != "" {
			c.buf.WriteString("[")
			c.buf.WriteString(url)
			c.buf.WriteString("](")
			c.buf.WriteString(url)
			c.buf.WriteString(")")
		}
	default:
		// Unknown inline: render children if any.
		if len(node.Content) > 0 {
			c.renderInlineChildren(node.Content, depth)
		}
		if node.Text != "" {
			c.buf.WriteString(node.Text)
		}
	}
}

// mediaMarkdown renders an ADF media node as a markdown image reference so an
// embedded image is never silently dropped. External media carry a direct URL;
// file/link media reference an attachment by filename (the node's alt text),
// which the document's "Attachments" section maps to a download URL. When no
// filename is available the media id is used so the reference stays traceable.
func mediaMarkdown(node ADFNode) string {
	alt := attrStr(node.Attrs, "alt")
	label := alt
	if label == "" {
		label = "image"
	}

	if attrStr(node.Attrs, "type") == "external" {
		if url := attrStr(node.Attrs, "url"); url != "" {
			return "![" + label + "](" + url + ")"
		}
	}

	src := alt
	if src == "" {
		src = attrStr(node.Attrs, "id")
	}
	return "![" + label + "](" + src + ")"
}

func applyMarks(text string, marks []ADFMark) string {
	for _, mark := range marks {
		switch mark.Type {
		case "strong":
			text = "**" + text + "**"
		case "em":
			text = "*" + text + "*"
		case "code":
			text = "`" + text + "`"
		case "strike":
			text = "~~" + text + "~~"
		case "link":
			href := ""
			if mark.Attrs != nil {
				if h, ok := mark.Attrs["href"].(string); ok {
					href = h
				}
			}
			text = "[" + text + "](" + href + ")"
		}
	}
	return text
}

func (c *converter) renderList(items []ADFNode, depth int, ordered bool) {
	for i, item := range items {
		prefix := "- "
		if ordered {
			prefix = fmt.Sprintf("%d. ", i+1)
		}
		indent := strings.Repeat("  ", depth)
		c.buf.WriteString(indent)
		c.buf.WriteString(prefix)
		c.renderListItemContent(item, depth)
	}
	if depth == 0 {
		c.buf.WriteString("\n")
	}
}

func (c *converter) renderListItem(node ADFNode, depth int) {
	c.renderListItemContent(node, depth)
}

func (c *converter) renderListItemContent(item ADFNode, depth int) {
	for i, child := range item.Content {
		switch child.Type {
		case "paragraph":
			c.renderInlineChildren(child.Content, depth)
			if i < len(item.Content)-1 {
				c.buf.WriteString("\n")
			} else {
				c.buf.WriteString("\n")
			}
		case "bulletList":
			if i == 0 {
				c.buf.WriteString("\n")
			}
			c.renderList(child.Content, depth+1, false)
		case "orderedList":
			if i == 0 {
				c.buf.WriteString("\n")
			}
			c.renderList(child.Content, depth+1, true)
		default:
			captured := c.captureNode(child, depth)
			c.buf.WriteString(strings.TrimRight(captured, "\n"))
			c.buf.WriteString("\n")
		}
	}
}

func (c *converter) renderPlainChildren(nodes []ADFNode) {
	for _, node := range nodes {
		if node.Text != "" {
			c.buf.WriteString(node.Text)
		}
		if len(node.Content) > 0 {
			c.renderPlainChildren(node.Content)
		}
	}
}

func (c *converter) renderTable(rows []ADFNode) {
	if len(rows) == 0 {
		return
	}

	var table [][]string
	for _, row := range rows {
		if row.Type != "tableRow" {
			continue
		}
		var cells []string
		for _, cell := range row.Content {
			content := c.captureNodes(cell.Content, 0)
			content = strings.TrimRight(content, "\n")
			content = strings.ReplaceAll(content, "\n", " ")
			cells = append(cells, content)
		}
		table = append(table, cells)
	}

	if len(table) == 0 {
		return
	}

	// Determine column count from the widest row.
	maxCols := 0
	for _, row := range table {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	// Write header row.
	c.writeTableRow(table[0], maxCols)

	// Write separator.
	c.buf.WriteString("|")
	for i := 0; i < maxCols; i++ {
		c.buf.WriteString(" --- |")
	}
	c.buf.WriteString("\n")

	// Write data rows.
	for _, row := range table[1:] {
		c.writeTableRow(row, maxCols)
	}
	c.buf.WriteString("\n")
}

func (c *converter) writeTableRow(cells []string, maxCols int) {
	c.buf.WriteString("|")
	for i := 0; i < maxCols; i++ {
		c.buf.WriteString(" ")
		if i < len(cells) {
			c.buf.WriteString(cells[i])
		}
		c.buf.WriteString(" |")
	}
	c.buf.WriteString("\n")
}

// captureNodes renders nodes into a temporary buffer and returns the result.
func (c *converter) captureNodes(nodes []ADFNode, depth int) string {
	saved := c.buf
	c.buf = strings.Builder{}
	c.renderNodes(nodes, depth)
	result := c.buf.String()
	c.buf = saved
	return result
}

func (c *converter) captureNode(node ADFNode, depth int) string {
	saved := c.buf
	c.buf = strings.Builder{}
	c.renderNode(node, depth, 0)
	result := c.buf.String()
	c.buf = saved
	return result
}

func panelLabel(panelType string) string {
	switch strings.ToLower(panelType) {
	case "info":
		return "**Info:**"
	case "note":
		return "**Note:**"
	case "warning":
		return "**Warning:**"
	case "error":
		return "**Error:**"
	case "success":
		return "**Success:**"
	default:
		return ""
	}
}

// SpliceSection replaces the content of a named heading section in doc with
// newNodes. The section runs from the matching heading node through the node
// before the next heading of equal or lower level (i.e. equal or smaller
// level number), or to the end of doc.Content. If the heading is not found,
// newNodes are appended to doc.Content. Returns a shallow copy; does not
// mutate the original.
func SpliceSection(doc *ADFDoc, heading string, newNodes []ADFNode) *ADFDoc {
	out := &ADFDoc{Type: doc.Type, Version: doc.Version}

	headingLower := strings.ToLower(strings.TrimSpace(heading))
	found := false
	skip := false
	foundLevel := 0

	for _, node := range doc.Content {
		if skip {
			// Stop skipping at the next heading of equal or higher hierarchy
			// (level <= foundLevel means same or broader section).
			if node.Type == "heading" {
				lvl := attrInt(node.Attrs, "level", 1)
				if lvl <= foundLevel {
					skip = false
					out.Content = append(out.Content, node)
				}
				// else: still inside the old section body, keep skipping
			}
			continue
		}

		if !found && node.Type == "heading" {
			// Extract plain text from the heading node.
			var sb strings.Builder
			for _, child := range node.Content {
				if child.Type == "text" {
					sb.WriteString(child.Text)
				}
			}
			if strings.ToLower(strings.TrimSpace(sb.String())) == headingLower {
				found = true
				foundLevel = attrInt(node.Attrs, "level", 1)
				// Emit the heading itself, then the new content.
				out.Content = append(out.Content, node)
				out.Content = append(out.Content, newNodes...)
				skip = true
				continue
			}
		}

		out.Content = append(out.Content, node)
	}

	if !found {
		// Section absent: append heading + content.
		headingNode := ADFNode{
			Type: "heading",
			Attrs: map[string]any{"level": 2},
			Content: []ADFNode{
				{Type: "text", Text: heading},
			},
		}
		out.Content = append(out.Content, headingNode)
		out.Content = append(out.Content, newNodes...)
	}

	return out
}

func attrStr(attrs map[string]any, key string) string {
	if attrs == nil {
		return ""
	}
	if v, ok := attrs[key].(string); ok {
		return v
	}
	return ""
}

func attrInt(attrs map[string]any, key string, defaultVal int) int {
	if attrs == nil {
		return defaultVal
	}
	switch v := attrs[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return defaultVal
	}
}
