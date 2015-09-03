package webui

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"testing"

	"github.com/go-martini/martini"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
)

func setupTest(t *testing.T) (*db.Handle, *martini.Martini) {
	feeds := make(map[string]*feedwatcher.FeedWatcher)
	dbh := db.NewMemoryDBHandle(false, true)
	authenticateUser = func(res http.ResponseWriter, req *http.Request, dbh *db.Handle) {
	}
	m := createMartini(dbh, feeds)
	return dbh, m
}

func failOnError(t *testing.T, err error) {
	if err != nil {
		fmt.Println(string(debug.Stack()))
		t.Fatalf("Error: %s", err.Error())
	}
}
