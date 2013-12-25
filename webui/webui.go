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
	"time"
)

type UserPageData struct {
	User  *db.User
	Feeds []db.FeedInfo
}

func feedsPage(rend render.Render, feeds map[string]*feed_watcher.FeedWatcher) {
	rend.JSON(http.StatusOK, feeds)
}

func userPage(r render.Render, params martini.Params, dbh *db.DbDispatcher) {
	glog.Infof("Got \"%s\" as email.", params["email"])
	u, err := dbh.GetUserByEmail(params["email"])
	if err != nil {
		r.JSON(404, err.Error())
		return
	}

	feeds, err := dbh.GetUsersFeeds(u)
	if err != nil {
		r.JSON(404, err.Error())
		return
	}

	r.JSON(200, UserPageData{
		User:  u,
		Feeds: feeds,
	})
}

func addFeed(rend render.Render, r *http.Request, w http.ResponseWriter, dbh *db.DbDispatcher) {
	name, url := r.FormValue("name"), r.FormValue("url")
	_, err := dbh.GetFeedByUrl(url)
	if err == nil {
		rend.JSON(http.StatusConflict, errors.New("Feed already exists"))
		return
	}
	feed, err := dbh.AddFeed(name, url)
	if err != nil {
		rend.JSON(http.StatusInternalServerError, fmt.Errorf("Couldn't create feed: %s", err))
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/feeds/%d", feed.Id))
	rend.JSON(http.StatusCreated, feed)
}

func getFeed(rend render.Render, dbh *db.DbDispatcher, params martini.Params) {
	feed_id, err := strconv.Atoi(params["id"])
	if err != nil {
		rend.JSON(
			http.StatusNotFound,
			fmt.Errorf("Feed with id \"%s\" does not exist\n.", params["id"]),
		)
	}
	feed, err := dbh.GetFeedById(feed_id)
	if err != nil {
	}
	rend.JSON(http.StatusOK, feed)
}

type SubscribeFeedPageData struct {
	User    *db.User
	FeedUrl string
}

func subscribeFeed(rend render.Render, r *http.Request, dbh *db.DbDispatcher) {
	user_email, feed_url := r.FormValue("useremail"), r.FormValue("feed_url")
	user, err := dbh.GetUserByEmail(user_email)

	err = dbh.AddFeedsToUser(user, []string{feed_url})
	if err != nil {
		return
	}
	rend.JSON(http.StatusOK, SubscribeFeedPageData{user, feed_url})
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC1123)
}

func createMartini(dbh *db.DbDispatcher, feeds map[string]*feed_watcher.FeedWatcher) *martini.ClassicMartini {
	m := martini.Classic()
	m.Use(render.Renderer(render.Options{
		IndentJson: true,
	}))

	m.Map(dbh)
	m.Map(feeds)

	// API
	// Feed API
	// All Feeds
	m.Get("/api/1/feeds", feedsPage)
	// One Feed
	m.Get("/api/1/feeds/:id", getFeed)
	// Add Feed
	m.Post("/api/1/feeds", addFeed)

	// User API
	m.Get("/api/1/user/:email", userPage)
	// Subscribe a user to a feed
	m.Post("/api/1/user/subscribe", subscribeFeed)
	// Unsubscribe a user from a feed
	//m.Delete("/user/unsubscribe/:feed_url", unsubscribeFeed)

	return m
}

func RunWebUi(config *config.Config, dbh *db.DbDispatcher, feeds map[string]*feed_watcher.FeedWatcher) {
	m := createMartini(dbh, feeds)
	glog.Fatal(http.ListenAndServe(config.WebServer.ListenAddress, m))
}
