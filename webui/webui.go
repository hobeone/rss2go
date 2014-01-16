package webui

import (
	"errors"
	"fmt"
	"github.com/codegangsta/martini"
	"github.com/golang/glog"
	"github.com/hobeone/martini-contrib/render"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"net/http"
	"strconv"
)

func handleError(err error) {
	if err != nil {
		panic(err.Error())
	}
}

func parseParamIds(str_ids []string) ([]int, error) {
	if len(str_ids) == 0 {
		return nil, errors.New("No ids given")
	}
	int_ids := make([]int, len(str_ids))
	for i, str_id := range str_ids {
		int_id, err := strconv.Atoi(str_id)
		if err != nil {
			return nil, fmt.Errorf("Error parsing feed id: %s", err)
		}
		int_ids[i] = int_id
	}
	return int_ids, nil
}

func createMartini(dbh *db.DbDispatcher, feeds map[string]*feed_watcher.FeedWatcher) *martini.Martini {
	m := martini.New()
	m.Use(martini.Logger())

	m.Use(
		render.Renderer(
			render.Options{
				IndentJSON: true,
			},
		),
	)
	m.Use(JSONRecovery())

	m.Map(dbh)
	m.Map(feeds)

	r := martini.NewRouter()
	// API
	// Feed API
	// All Feeds or multiple feeds (ids= parameter)
	r.Get("/api/1/feeds", getFeeds)
	// One Feed
	r.Get("/api/1/feeds/:id", getFeed)
	// Update
	r.Put("/api/1/feeds/:id", updateFeed)
	// Add Feed
	r.Post("/api/1/feeds", addFeed)
	r.Delete("/api/1/feeds/:id", deleteFeed)

	// User API
	r.Get("/api/1/users", getUsers)
	r.Get("/api/1/users/:id", getUser)
	r.Put("/api/1/users/:id", updateUser)
	r.Post("/api/1/users", addUser)
	r.Delete("/api/1/users/:id", deleteUser)

	// Subscribe a user to a feed
	//r.Put("/api/1/users/:user_id/subscribe/:feed_id", subscribeFeed)
	// Unsubscribe a user from a feed
	//m.Delete("/api/1/users/:user_id/unsubscribe/:feed_id", unsubscribeFeed)

	m.Action(r.Handle)

	return m
}

func RunWebUi(config *config.Config, dbh *db.DbDispatcher, feeds map[string]*feed_watcher.FeedWatcher) {
	m := createMartini(dbh, feeds)
	glog.Fatal(http.ListenAndServe(config.WebServer.ListenAddress, m))
}
