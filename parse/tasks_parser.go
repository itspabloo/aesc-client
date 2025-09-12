package parse

import (
	"io"
	"strings"
	"github.com/PuerkitoBio/goquery"
)

type Problem struct {
	Name string
	URL string
}

func ParseProblems(r io.Reader) ([]Problem, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}
	var problems []Problem
	doc.Find("ul.menu a[href*='problem']").Each(func(i int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			return
		}
		name := strings.TrimSpace(s.Text())
		if name != "" {
			problems = append(problems, Problem{ Name: name, URL: href, })
		}
	})
	return problems, nil
}
