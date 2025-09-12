package parse

import (
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

func FetchStatement(client *http.Client, baseURL, startPath, outDir string) error {
	if client == nil {
		return fmt.Errorf("nil http client")
	}
	startURL := resolveRelativeURL(baseURL, startPath)
	resp, err := client.Get(startURL)
	if err != nil {
		return fmt.Errorf("GET %s: %w", startURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("GET %s returned %s", startURL, resp.Status)
	}
	doc, err := html.Parse(resp.Body)
	if err != nil {
		return fmt.Errorf("parse %s: %w", startURL, err)
	}
	menu := locateMenu(doc)
	if menu == nil {
		return fmt.Errorf("menu not found on %s", startURL)
	}
	contestHref := findFirstHref(menu, func(h string) bool { return strings.Contains(h, "ranking-table") })
	if contestHref == "" {
		return fmt.Errorf("contest link not found on %s", startURL)
	}
	contestURL := resolveRelativeURL(startURL, contestHref)
	resp2, err := client.Get(contestURL)
	if err != nil {
		return fmt.Errorf("GET %s: %w", contestURL, err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode >= 400 {
		return fmt.Errorf("GET %s returned %s", contestURL, resp2.Status)
	}
	doc2, err := html.Parse(resp2.Body)
	if err != nil {
		return fmt.Errorf("parse %s: %w", contestURL, err)
	}
	problemHref := findFirstHref(doc2, func(h string) bool { return strings.Contains(h, "/cs/problem") })
	if problemHref == "" {
		return fmt.Errorf("problem link not found on %s", contestURL)
	}
	problemURL := resolveRelativeURL(contestURL, problemHref)
	resp3, err := client.Get(problemURL)
	if err != nil {
		return fmt.Errorf("GET %s: %w", problemURL, err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode >= 400 {
		return fmt.Errorf("GET %s returned %s", problemURL, resp3.Status)
	}
	doc3, err := html.Parse(resp3.Body)
	if err != nil {
		return fmt.Errorf("parse %s: %w", problemURL, err)
	}
	iframeSrc, ok := findIframeSrc(doc3)
	var contentRoot *html.Node
	var baseForResolve string
	if ok {
		iframeURL := resolveRelativeURL(problemURL, iframeSrc)
		resp4, err := client.Get(iframeURL)
		if err != nil {
			return fmt.Errorf("GET %s: %w", iframeURL, err)
		}
		defer resp4.Body.Close()
		if resp4.StatusCode >= 400 {
			return fmt.Errorf("GET %s returned %s", iframeURL, resp4.Status)
		}
		doc4, err := html.Parse(resp4.Body)
		if err != nil {
			return fmt.Errorf("parse %s: %w", iframeURL, err)
		}
		contentRoot = doc4
		baseForResolve = iframeURL
	} else {
		contentRoot = doc3
		baseForResolve = problemURL
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outDir, err)
	}
	imagesDir := filepath.Join(outDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", imagesDir, err)
	}
	outFile := filepath.Join(outDir, "statement.txt")
	f, err := os.Create(outFile)
	if err != nil {
		return fmt.Errorf("create %s: %w", outFile, err)
	}
	defer f.Close()
	if err := extractTextWithFormulas(client, contentRoot, baseForResolve, imagesDir, f); err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	return nil
}

func locateMenu(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, "ul") {
		for _, a := range n.Attr {
			if strings.EqualFold(a.Key, "class") && strings.Contains(a.Val, "menu") {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if res := locateMenu(c); res != nil {
			return res
		}
	}
	return nil
}

func findFirstHref(n *html.Node, pred func(string) bool) string {
	if n == nil {
		return ""
	}
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, "a") {
		for _, a := range n.Attr {
			if strings.EqualFold(a.Key, "href") {
				if pred(a.Val) {
					return a.Val
				}
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if res := findFirstHref(c, pred); res != "" {
			return res
		}
	}
	return ""
}

func findIframeSrc(n *html.Node) (string, bool) {
	if n == nil {
		return "", false
	}
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, "iframe") {
		for _, a := range n.Attr {
			if strings.EqualFold(a.Key, "src") {
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
					savePath := filepath.Join(imagesDir, name)
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
			return err
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

func resolveRelativeURL(base, href string) string {
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

func normalizeTeX(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if strings.HasPrefix(s, "$$") && strings.HasSuffix(s, "$$") && len(s) > 4 {
		inner := strings.TrimSpace(s[2 : len(s)-2])
		return inner, true
	}
	if strings.HasPrefix(s, `\(`) && strings.HasSuffix(s, `\)`) && len(s) > 4 {
		inner := strings.TrimSpace(s[2 : len(s)-2])
		return inner, false
	}
	if strings.HasPrefix(s, `\[` ) && strings.HasSuffix(s, `\]`) && len(s) > 4 {
		inner := strings.TrimSpace(s[2 : len(s)-2])
		return inner, true
	}
	if strings.HasPrefix(s, "$") && strings.HasSuffix(s, "$") && len(s) > 2 {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		return inner, false
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

