package tgbot

import (
	"html"
	"strings"

	"mikan/internal/tgapi"
)

// The look of the admin's cards. Like admin_ui.go these are pure functions of their arguments:
// every piece of data is escaped here, and the markup is only what this file writes. Telegram
// gives a text bot a small set of tools, and the cards use all of them: a quote block (the
// vertical bar) groups what belongs together, a monospace line holds a gauge, bold carries the
// values and the title alone, and the buttons are coloured by what they do.

// Row is one line of a block: a mark, a label and a value. The label is plain, the value bold,
// so the eye reads down the values.
type Row struct {
	Icon, Label, Value string
}

// block is a group of rows in a quote block, with an optional caption in bold above it:
//
//	▎ Срок            01.03.2027
//	▎ Тариф           Безлимит
func block(caption string, rows ...Row) string {
	var lines []string
	if caption != "" {
		lines = append(lines, "<b>"+html.EscapeString(caption)+"</b>")
	}
	for _, r := range rows {
		if r.Label == "" && r.Value == "" {
			continue
		}
		var b strings.Builder
		if r.Icon != "" {
			b.WriteString(html.EscapeString(r.Icon) + " ")
		}
		if r.Label != "" {
			b.WriteString(html.EscapeString(r.Label))
			if r.Value != "" {
				b.WriteString(": ")
			}
		}
		if r.Value != "" {
			b.WriteString("<b>" + html.EscapeString(r.Value) + "</b>")
		}
		lines = append(lines, b.String())
	}
	if len(lines) == 0 {
		return ""
	}
	return "<blockquote>" + strings.Join(lines, "\n") + "</blockquote>"
}

// gauge draws a label, a bar and a share in one monospace line, so the bars of two cards line
// up: «Трафик  ▰▰▱▱▱▱▱▱▱▱ 20%».
func gauge(label string, used, limit int64) string {
	return "<code>" + html.EscapeString(label) + "  " + bar(used, limit) + "</code>"
}

// clr paints a button: it keeps the button as it is and sets only the colour.
func clr(style string, b tgapi.Button) tgapi.Button {
	b.Style = style
	return b
}

// tint paints every button of a keyboard row.
func tint(style string, row ...tgapi.Button) []tgapi.Button {
	for i := range row {
		row[i].Style = style
	}
	return row
}

// join puts the parts of a card together, one blank line between them, skipping the empty.
func join(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}
