package webui

import (
	"fmt"
	"github.com/codegangsta/martini"
	"github.com/golang/glog"
	"github.com/hobeone/martini-contrib/render"
	"net/http"
)

// Recovery returns a middleware that recovers from any panics and writes a 500 if there was one.
func JSONRecovery() martini.Handler {
	return func(r render.Render, c martini.Context) {
		defer func() {
			if err := recover(); err != nil {
				r.JSON(http.StatusInternalServerError,
					fmt.Sprintf("Error processing request: %v", err))
				glog.Errorf("PANIC: %s", err)
			}
		}()
		c.Next()
	}
}
