package main

import (
	"bytes"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/SlyMarbo/rss"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type article struct {
	ID    int64
	Title string
	URL   string
	Links []*link
	Feed  string
	Date  time.Time
}

type link struct {
	ID      int64
	URL     string
	Domain  string
	Context string

	Queued   bool      // populated from DB
	QueuedAt time.Time // populated from DB
}

type visitor interface {
	visit(n *html.Node) visitor
}

func walkHTML(n *html.Node, v visitor) {
	vv := v.visit(n)
	if vv == nil {
		return
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		walkHTML(child, vv)
	}
	v.visit(nil)
}

func attr(n *html.Node, name string) string {
	for _, at := range n.Attr {
		if strings.ToLower(at.Key) == name {
			return at.Val
		}
	}
	return ""
}

type flattenVisitor struct {
	out bytes.Buffer
}

func (v *flattenVisitor) visit(n *html.Node) visitor {
	if n == nil {
		return nil
	}
	if n.Type == html.TextNode {
		v.out.Write([]byte(n.Data + " "))
	}
	return v
}

func flatten(n *html.Node) string {
	var v flattenVisitor
	walkHTML(n, &v)
	return v.out.String()
}

type byDate []*article

func (xs byDate) Len() int           { return len(xs) }
func (xs byDate) Swap(i, j int)      { xs[i], xs[j] = xs[j], xs[i] }
func (xs byDate) Less(i, j int) bool { return xs[i].Date.Before(xs[j].Date) }

type linkVisitor struct {
	links []*link
}

func (v *linkVisitor) visit(n *html.Node) visitor {
	if n == nil {
		return nil
	}
	if n.Type == html.ElementNode && n.DataAtom == atom.A {
		if href := attr(n, "href"); href != "" {
			if url, err := url.Parse(href); err == nil {
				v.links = append(v.links, &link{
					URL:     href,
					Domain:  url.Host,
					Context: flatten(n.Parent),
				})
			}
		}
	}
	return v
}

func findLinks(s string) ([]*link, error) {
	root, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return nil, err
	}

	var v linkVisitor
	walkHTML(root, &v)
	return v.links, nil
}

func fetch(urlstr string) ([]*article, error) {
	feed, err := rss.Fetch(urlstr)
	if err != nil {
		return nil, err
	}

	feedurl, err := url.Parse(feed.Link)
	if err != nil {
		return nil, err
	}

	var all []*article
	for _, item := range feed.Items {
		links, err := findLinks(item.Content)
		if err != nil {
			log.Printf("%s: %v\n", item.Title, err)
		}

		var filtered []*link
		for _, link := range links {
			linkurl, err := url.Parse(link.URL)
			if err == nil && linkurl.Host == feedurl.Host {
				//log.Println("ignoring", link.URL)
				continue
			}
			filtered = append(filtered, link)
		}

		if len(links) > 0 {
			all = append(all, &article{
				Title: item.Title,
				URL:   item.Link,
				Links: filtered,
				Feed:  feed.Title,
				Date:  item.Date,
			})
		}
	}
	return all, nil
}
