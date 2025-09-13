package parse

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const MaxLineWidth = 180

func FetchStatementToString(client *http.Client, problemURL string) (string, error) {
	if client == nil {
		return "", fmt.Errorf("nil http client")
	}
	resp, err := client.Get(problemURL)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", problemURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("GET %s returned %s", problemURL, resp.Status)
	}
	root, err := html.Parse(resp.Body)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", problemURL, err)
	}
	iframeSrc, ok := findIframeSrcPrefer(root)
	var contentRoot *html.Node
	if ok {
		iframeURL := resolveRelativeURL(problemURL, iframeSrc)
		resp2, err := client.Get(iframeURL)
		if err != nil {
			return "", fmt.Errorf("GET %s: %w", iframeURL, err)
		}
		defer resp2.Body.Close()
		if resp2.StatusCode >= 400 {
			return "", fmt.Errorf("GET %s returned %s", iframeURL, resp2.Status)
		}
		root2, err := html.Parse(resp2.Body)
		if err != nil {
			return "", fmt.Errorf("parse %s: %w", iframeURL, err)
		}
		contentRoot = root2
	} else {
		contentRoot = root
	}
	var buf bytes.Buffer
	if err := extractTextWithFormulas(contentRoot, &buf); err != nil {
		return "", fmt.Errorf("extract text: %w", err)
	}
	cleaned := cleanExtracted(buf.String())
	out := wrapLines(cleaned, MaxLineWidth)
	return out, nil
}

func findIframeSrcPrefer(n *html.Node) (string, bool) {
	if n == nil {
		return "", false
	}
	idRe := regexp.MustCompile(`(?i)^aid\d+pid\d+$`)
	var candidate string
	var found bool
	var f func(*html.Node)
	f = func(x *html.Node) {
		if x == nil || found {
			return
		}
		if x.Type == html.ElementNode && strings.EqualFold(x.Data, "iframe") {
			var id, src string
			for _, a := range x.Attr {
				k := strings.ToLower(a.Key)
				switch (k) {
					case "id":
						id = a.Val
					case "src":
						src = a.Val
				}
			}
			if src == "" {
				// skip
			} else if idRe.MatchString(id) {
				candidate = src
				found = true
				return
			} else if strings.Contains(src, "text-pack") {
				if candidate == "" {
					candidate = src
				}
			} else if candidate == "" {
				candidate = src
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			if found {
				return
			}
			f(c)
		}
	}
	f(n)
	if candidate == "" {
		return "", false
	}
	return candidate, true
}

func resolveRelativeURL(base, href string) string {
	href = strings.TrimSpace(href)
	u, err := url.Parse(href)
	if err == nil && u.IsAbs() {
		return href
	}
	baseu, err := url.Parse(base)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return baseu.ResolveReference(ref).String()
}

func extractTextWithFormulas(root *html.Node, w io.Writer) error {
	if root == nil {
		return nil
	}
	wsRe := regexp.MustCompile(`\s+`)
	appendInline := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		s = wsRe.ReplaceAllString(s, " ")
		io.WriteString(w, s+" ")
	}
	var walker func(*html.Node) error
	walker = func(n *html.Node) error {
		if n == nil {
			return nil
		}
		if n.Type == html.TextNode {
			appendInline(n.Data)
			return nil
		}
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "p", "div", "section", "article":
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if err := walker(c); err != nil {
						return err
					}
				}
				io.WriteString(w, "\n\n")
				return nil
			case "br":
				io.WriteString(w, "\n")
				return nil
			case "ul", "ol":
				for li := n.FirstChild; li != nil; li = li.NextSibling {
					if li.Type == html.ElementNode && strings.EqualFold(li.Data, "li") {
						io.WriteString(w, "- ")
						if err := walker(li); err != nil {
							return err
						}
						io.WriteString(w, "\n")
					}
				}
				return nil
			case "img":
				var alt string
				for _, a := range n.Attr {
					if strings.EqualFold(a.Key, "alt") {
						alt = a.Val
					}
				}
				if alt != "" {
					appendInline(alt)
				} else {
					appendInline("[IMAGE]")
				}
				return nil
			case "script":
				var typ string
				for _, a := range n.Attr {
					if strings.EqualFold(a.Key, "type") {
						typ = a.Val
						break
					}
				}
				if strings.Contains(strings.ToLower(typ), "math") {
					raw := extractAllText(n)
					clean, _ := normalizeTeX(raw)
					if clean != "" {
						clean = simplifyPowers(clean)
						clean = inlineTexToPlain(clean)
						appendInline(clean)
					}
					return nil
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := walker(c); err != nil {
				return err
			}
		}
		return nil
	}
	return walker(root)
}

func extractAllText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var f func(*html.Node)
	f = func(x *html.Node) {
		if x == nil {
			return
		}
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return b.String()
}

func normalizeTeX(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "$$") && strings.HasSuffix(s, "$$") {
		return strings.TrimSpace(s[2 : len(s)-2]), true
	}
	if strings.HasPrefix(s, "$") && strings.HasSuffix(s, "$") {
		return strings.TrimSpace(s[1 : len(s)-1]), false
	}
	if strings.HasPrefix(s, `\(`) && strings.HasSuffix(s, `\)`) {
		return strings.TrimSpace(s[2 : len(s)-2]), false
	}
	if strings.HasPrefix(s, `\[` ) && strings.HasSuffix(s, `\]`) {
		return strings.TrimSpace(s[2 : len(s)-2]), true
	}
	reTrim := regexp.MustCompile(`^\s*(?:<!--.*?-->\s*)*(.*?)(?:\s*<!--.*?-->\s*)*\s*$`)
	out := reTrim.ReplaceAllString(s, "$1")
	return strings.TrimSpace(out), false
}

func simplifyPowers(tex string) string {
	reCurly := regexp.MustCompile(`\^\{([0-9]+)\}`)
	tex = reCurly.ReplaceAllString(tex, "^$1")
	return tex
}

func inlineTexToPlain(tex string) string {
	tex = strings.ReplaceAll(tex, `\cdot`, "*")
	tex = strings.ReplaceAll(tex, `\times`, "x")
	tex = strings.ReplaceAll(tex, `\le`, "<=")
	tex = strings.ReplaceAll(tex, `\ge`, ">=")
	tex = strings.ReplaceAll(tex, `\ldots`, "...")
	tex = strings.ReplaceAll(tex, `\;`, " ")
	return tex
}

func cleanExtracted(s string) string {
	if s == "" {
		return ""
	}
	reBigComment := regexp.MustCompile(`(?is)<!--\s*/\*\s*Font Definitions\b.*?-->`)
	s = reBigComment.ReplaceAllString(s, "")
	reMso := regexp.MustCompile(`(?is)<!--.*?mso.*?-->`)
	s = reMso.ReplaceAllString(s, "")
	reComment := regexp.MustCompile(`(?s)<!--.*?-->`)
	s = reComment.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	var out []string
	reTrashLine := regexp.MustCompile(`(?i)^\s*(none|html|<[^>]+>|\s*/\*.*|\*.*\*/|@page|font-family|Font Definitions|\{|\}).*`)
	for i := range len(lines) {
		ln := strings.TrimSpace(lines[i])
		if ln == "" {
			if len(out) == 0 || out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		if reTrashLine.MatchString(ln) {
			continue
		}
		out = append(out, ln)
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	if len(out) == 0 {
		return ""
	}
	headings := regexp.MustCompile(`(?i)^(Задача|Входные данные|Входные данные:|Выходные данные|Выходные данные:|Примеры|Примеры входных данных|Примеры:|Примечание|Ограничение времени|Ограничения)$`)
	var spaced []string
	for i := 0; i < len(out); i++ {
		ln := out[i]
		if headings.MatchString(ln) {
			if len(spaced) > 0 && spaced[len(spaced)-1] != "" {
				spaced = append(spaced, "")
			}
			spaced = append(spaced, ln)
			spaced = append(spaced, "")
		} else {
			spaced = append(spaced, ln)
		}
	}
	for len(spaced) > 0 && spaced[len(spaced)-1] == "" {
		spaced = spaced[:len(spaced)-1]
	}
	return strings.Join(spaced, "\n")
}

func wrapLines(s string, width int) string {
	if width <= 0 {
		width = MaxLineWidth
	}
	lines := strings.Split(s, "\n")
	var outLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			outLines = append(outLines, "")
			continue
		}
		words := strings.Fields(line)
		var cur strings.Builder
		curLen := 0
		for _, w := range words {
			if curLen == 0 {
				cur.WriteString(w)
				curLen = len(w)
			} else if curLen+1+len(w) <= width {
				cur.WriteByte(' ')
				cur.WriteString(w)
				curLen += 1 + len(w)
			} else {
				outLines = append(outLines, cur.String())
				cur.Reset()
				cur.WriteString(w)
				curLen = len(w)
			}
		}
		if cur.Len() > 0 {
			outLines = append(outLines, cur.String())
		}
	}
	return strings.Join(outLines, "\n")
}

