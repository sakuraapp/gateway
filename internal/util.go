package internal

import (
	"net/url"
	"strings"
)

func GetDomain(url *url.URL) string {
	parts := strings.Split(url.Hostname(), ".")
	domain := strings.Join(parts[len(parts) - 2:], ".")

	return domain
}