package parse

import (
	"io"
	"strings"
	"golang.org/x/net/html"
)

type Contest struct {
	Name string
	URL string
}

func ParseContests(r io.Reader) ([]Contest, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	menu := findMenu(doc)
	if menu == nil {
		return []Contest{}, nil
	}

	var contests []Contest
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := ""
			for _, a := range n.Attr {
				if a.Key == "href" {
					href = a.Val
					break
				}
			}
			if href != "" && strings.Contains(href, "ranking-table") {
				text := strings.TrimSpace(extractText(n))
				if text != "" {
					contests = append(contests, Contest{
						Name: text,
						URL:  href,
					})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(menu)
	return contests, nil
}

func findMenu(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "ul" {
		for _, a := range n.Attr {
			if a.Key == "class" && strings.Contains(a.Val, "menu") {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if res := findMenu(c); res != nil {
			return res
		}
	}
	return nil
}

func extractText(n *html.Node) string {
	var b strings.Builder
	var f func(*html.Node)
	f = func(x *html.Node) {
		if x.Type == html.TextNode {
			txt := strings.TrimSpace(x.Data)
			if txt != "" {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(txt)
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return b.String()
}
