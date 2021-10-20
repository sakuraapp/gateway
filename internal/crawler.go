package internal

import (
	"github.com/sakuraapp/shared/resource"
	"golang.org/x/net/html"
	"net/http"
)

var iconSelectors = map[string]bool{
	"apple-touch-icon-precomposed": true,
	"apple-touch-icon": true,
	"shortcut icon": true,
	"icon": true,
}

type Crawler struct {
	transport http.RoundTripper
}

func NewCrawler() *Crawler {
	return &Crawler{
		transport: http.DefaultTransport,
	}
}

func (c *Crawler) Get(url string) (*resource.MediaItemInfo, error) {
	client := &http.Client{Transport: c.transport}
	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return nil, err
	}

	req.Header.Add("User-agent", "Googlebot/2.1 (+http://www.google.com/bot.html)")

	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}

	info := &resource.MediaItemInfo{Url: url}
	z := html.NewTokenizer(resp.Body)

	titleFound := false
	title := false

	iconFound := false
	icon := false

	loop:
	for {
		tt := z.Next()

		switch tt {
		case html.ErrorToken:
			err = z.Err()
			break loop
		case html.StartTagToken:
			t := z.Token()

			if t.Data == "title" {
				titleFound = true
			}

			if t.Data == "meta" {
				for _, attr := range t.Attr {
					if attr.Key == "title" {
						info.Title = attr.Val
						title = true
					}

					if attr.Key == "itemprop" && attr.Val == "image" {
						iconFound = true
					}

					if attr.Key == "content" && iconFound {
						info.Icon = attr.Val
						icon = true
						iconFound = false
						break
					}
				}
			}

			if t.Data == "link" {
				for _, attr := range t.Attr {
					if attr.Key == "rel" && iconSelectors[attr.Val] {
						iconFound = true
					}

					if attr.Key == "href" && iconFound {
						info.Icon = attr.Val
						icon = true
						iconFound = false
						break
					}
				}
			}

			iconFound = false

			if title && icon {
				return info, nil
			}
		case html.TextToken:
			if titleFound && !title {
				info.Title = z.Token().Data

				titleFound = false
				title = true
			}
		}
	}

	return info, err
}
