package tgbot

import (
	"encoding/json"
	"strings"
	"testing"

	"mikan/internal/tgapi"
)

// A block is a quote with the rows in it, the data escaped, an empty row left out.
func TestBlock(t *testing.T) {
	got := block("Cap <x>", Row{"🕒", "Срок", "01.03.2027"}, Row{Label: "", Value: ""}, Row{Label: "Имя", Value: "<b>&"})
	want := "<blockquote><b>Cap &lt;x&gt;</b>\n🕒 Срок: <b>01.03.2027</b>\nИмя: <b>&lt;b&gt;&amp;</b></blockquote>"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if block("only a caption") == "" || block("") != "" {
		t.Error("a block with a caption only is still a block; one with nothing is empty")
	}
}

// The gauge keeps the bar and puts the label in the same monospace line.
func TestGauge(t *testing.T) {
	g := gauge("Трафик", 20, 100)
	if !strings.HasPrefix(g, "<code>Трафик  ") || !strings.HasSuffix(g, "20%</code>") || strings.Count(g, "▰") != 2 {
		t.Fatalf("gauge: %s", g)
	}
	if g := gauge("<", 0, 0); !strings.Contains(g, "&lt;") || !strings.Contains(g, "∞") {
		t.Fatalf("unlimited or unescaped: %s", g)
	}
}

// The colour reaches the wire as «style», and a button without one sends nothing for it.
func TestButtonStyleOnTheWire(t *testing.T) {
	plain, _ := json.Marshal(tgapi.Button{Text: "a", CallbackData: "x"})
	if strings.Contains(string(plain), "style") {
		t.Errorf("an unpainted button sends a style: %s", plain)
	}
	red, _ := json.Marshal(clr(tgapi.StyleDanger, tgapi.Button{Text: "a", CallbackData: "x"}))
	if !strings.Contains(string(red), `"style":"danger"`) {
		t.Errorf("a painted button: %s", red)
	}
	row := tint(tgapi.StyleSuccess, tgapi.Button{Text: "1"}, tgapi.Button{Text: "2"})
	if row[0].Style != "success" || row[1].Style != "success" {
		t.Errorf("tint: %+v", row)
	}
}

func TestJoin(t *testing.T) {
	if got := join("a", "", "b"); got != "a\n\nb" {
		t.Errorf("join: %q", got)
	}
}

// htmlBalanced says whether the tags of a Telegram text open and close in order, with only the
// tags Telegram knows. A text that does not is refused by Telegram whole.
func htmlBalanced(s string) (bool, string) {
	allowed := map[string]bool{"b": true, "i": true, "u": true, "s": true, "code": true, "pre": true, "blockquote": true, "tg-spoiler": true, "a": true}
	var stack []string
	for i := 0; i < len(s); i++ {
		if s[i] != '<' {
			continue
		}
		j := strings.IndexByte(s[i:], '>')
		if j < 0 {
			return false, "unclosed <"
		}
		tag := s[i+1 : i+j]
		i += j
		closing := strings.HasPrefix(tag, "/")
		name := strings.TrimPrefix(tag, "/")
		if k := strings.IndexAny(name, " \t"); k >= 0 {
			name = name[:k]
		}
		if !allowed[name] {
			return false, "tag " + name
		}
		if !closing {
			stack = append(stack, name)
			continue
		}
		if len(stack) == 0 || stack[len(stack)-1] != name {
			return false, "closing " + name
		}
		stack = stack[:len(stack)-1]
	}
	if len(stack) != 0 {
		return false, "unclosed " + stack[len(stack)-1]
	}
	return true, ""
}

func TestHTMLBalanced(t *testing.T) {
	for s, want := range map[string]bool{
		"<b>a</b>": true, "<blockquote><b>a</b>\n<code>x</code></blockquote>": true,
		"<b>a": false, "<b><i>a</b></i>": false, "<div>a</div>": false, "a <": false,
	} {
		if got, _ := htmlBalanced(s); got != want {
			t.Errorf("%q: %v", s, got)
		}
	}
}
