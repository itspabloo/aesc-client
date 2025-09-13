package parse

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const MaxLineWidth = 170

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
	iframeSrc, ok := findIframeSrc(root)
	var contentRoot *html.Node
	var baseForResolve string
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
		baseForResolve = iframeURL
	} else {
		contentRoot = root
		baseForResolve = problemURL
	}
	var buf bytes.Buffer
	if err := extractTextWithFormulas(client, contentRoot, baseForResolve, "", &buf); err != nil {
		return "", fmt.Errorf("extract text: %w", err)
	}
	cleaned := cleanExtracted(buf.String())
	out := wrapLines(cleaned, MaxLineWidth)
	return out, nil
}

func findIframeSrc(n *html.Node) (string, bool) {
	if n == nil {
		return "", false
	}
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, "iframe") {
		for _, a := range n.Attr {
			if strings.EqualFold(a.Key, "src") && strings.TrimSpace(a.Val) != "" {
				return a.Val, true
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if src, ok := findIframeSrc(c); ok {
			return src, true
		}
	}
	return "", false
}

func resolveRelativeURL(base, href string) string {
	u, err := url.Parse(strings.TrimSpace(href))
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

func extractTextWithFormulas(client *http.Client, root *html.Node, baseForResolve, imagesDir string, w io.Writer) error {
	if root == nil {
		return nil
	}
	wsRe := regexp.MustCompile(`\s+`)
	imageCounter := 0
	appendText := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		s = wsRe.ReplaceAllString(s, " ")
		io.WriteString(w, s+"\n")
	}
	var walker func(*html.Node) error
	walker = func(n *html.Node) error {
		if n == nil {
			return nil
		}
		if n.Type == html.TextNode {
			appendText(n.Data)
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
				io.WriteString(w, "\n")
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
				var src, alt string
				for _, a := range n.Attr {
					if strings.EqualFold(a.Key, "src") {
						src = a.Val
					}
					if strings.EqualFold(a.Key, "alt") {
						alt = a.Val
					}
				}
				if src != "" {
					imageCounter++
					resolved := resolveRelativeURL(baseForResolve, src)
					ext := path.Ext(resolved)
					if ext == "" {
						ext = ".png"
					}
					name := fmt.Sprintf("formula_%03d%s", imageCounter, ext)
					savePath := name
					if imagesDir != "" {
						if err := os.MkdirAll(imagesDir, 0o755); err == nil {
							savePath = filepath.Join(imagesDir, name)
						}
					}
					_ = downloadToFile(client, resolved, savePath)
					io.WriteString(w, fmt.Sprintf("[IMAGE: %s]\n", savePath))
					if alt != "" {
						io.WriteString(w, fmt.Sprintf("Alt: %s\n", strings.TrimSpace(alt)))
					}
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
					clean, display := normalizeTeX(raw)
					if clean != "" {
						if display {
							io.WriteString(w, fmt.Sprintf("$$%s$$\n", clean))
						} else {
							io.WriteString(w, fmt.Sprintf("$%s$\n", clean))
						}
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

func downloadToFile(client *http.Client, srcURL, dest string) error {
	if client == nil {
		return fmt.Errorf("nil client")
	}
	resp, err := client.Get(srcURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("GET %s returned %s", srcURL, resp.Status)
	}
	destDir := filepath.Dir(dest)
	if destDir != "" && destDir != "." {
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", destDir, err)
		}
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func normalizeTeX(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if strings.HasPrefix(s, "$$") && strings.HasSuffix(s, "$$") && len(s) > 4 {
		return strings.TrimSpace(s[2 : len(s)-2]), true
	}
	if strings.HasPrefix(s, `\[` ) && strings.HasSuffix(s, `\]`) && len(s) > 4 {
		return strings.TrimSpace(s[2 : len(s)-2]), true
	}
	if strings.HasPrefix(s, `\(`) && strings.HasSuffix(s, `\)`) && len(s) > 4 {
		return strings.TrimSpace(s[2 : len(s)-2]), false
	}
	if strings.HasPrefix(s, "$") && strings.HasSuffix(s, "$") && len(s) > 2 {
		return strings.TrimSpace(s[1 : len(s)-1]), false
	}
	reTrim := regexp.MustCompile(`^\s*(?:<!--.*?-->\s*)*(.*?)(?:\s*<!--.*?-->\s*)*\s*$`)
	out := reTrim.ReplaceAllString(s, "$1")
	return strings.TrimSpace(out), false
}

func extractAllText(n *html.Node) string {
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
	return strings.TrimSpace(b.String())
}

func cleanExtracted(s string) string {
	reComment := regexp.MustCompile(`(?s)<!--.*?-->`)
	s = reComment.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	var out []string
	reTrashLine := regexp.MustCompile(`(?i)^\s*(none|html|<[^>]+>|\s*/\*.*|\*.*\*/).*$`)
	for _, ln := range lines {
		trim := strings.TrimSpace(ln)
		if trim == "" {
			if len(out) == 0 || out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		if reTrashLine.MatchString(trim) {
			continue
		}
		if strings.Contains(trim, "Font Definitions") || strings.Contains(trim, "font-family") || strings.Contains(trim, "{") || strings.Contains(trim, "}") {
			continue
		}
		out = append(out, trim)
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	return strings.Join(out, "\n")
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

