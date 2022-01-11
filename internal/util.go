package internal

import (
	"github.com/lesismal/nbio/nbhttp"
	"github.com/lesismal/nbio/taskpool"
	"net/url"
	"runtime"
	"strings"
)

func GetDomain(url *url.URL) string {
	parts := strings.Split(url.Hostname(), ".")
	domain := strings.Join(parts[len(parts) - 2:], ".")

	return domain
}

func NewTaskpool(conf *nbhttp.Config) *taskpool.MixedPool {
	if conf.MessageHandlerPoolSize <= 0 {
		conf.MessageHandlerPoolSize = runtime.NumCPU() * 1024
	}

	nativeSize := conf.MessageHandlerPoolSize - 1
	pool := taskpool.NewMixedPool(nativeSize, 1, 1024*1024)

	return pool
}