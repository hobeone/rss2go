package webui

import (
	"encoding/json"
	"fmt"
	"github.com/codegangsta/martini"
	"github.com/golang/glog"
	"github.com/hobeone/martini-contrib/render"
	"github.com/hobeone/rss2go/db"
	"net/http"
	"strconv"
)

type FeedsJSON struct {
	Feeds []FeedJSONItem `json:"feeds"`
}

type FeedJSONItem struct {
	db.FeedInfo
}

type FeedJSON struct {
	Feed *db.FeedInfo `json:"feed"`
}

func getFeeds(rend render.Render, r *http.Request, params martini.Params, dbh *db.DbDispatcher) {
	if err := r.ParseForm(); err != nil {
		rend.JSON(500, fmt.Errorf("Couldn't parse request: %s", err.Error()))
		return
	}
	var feed_json []FeedJSONItem
	if len(r.Form["ids[]"]) > 0 {
		feed_ids, err := parseParamIds(r.Form["ids[]"])
		handleError(err)
		feed_json = make([]FeedJSONItem, len(feed_ids))
		for i, feed_id := range feed_ids {
			feed, err := dbh.GetFeedById(feed_id)
			handleError(err)
			feed_json[i] = FeedJSONItem{*feed}
		}
		glog.Infof("Got %d feeds", len(feed_json))
	} else {
		feeds, err := dbh.GetAllFeeds()
		handleError(err)
		feed_json = make([]FeedJSONItem, len(feeds))
		for i, feed := range feeds {
			feed_json[i] = FeedJSONItem{feed}
		}
		glog.Infof("Got %d feeds", len(feed_json))
	}
	rend.JSON(http.StatusOK, FeedsJSON{feed_json})
}

func getFeed(rend render.Render, dbh *db.DbDispatcher, params martini.Params) {
	feed_id, err := strconv.Atoi(params["id"])
	handleError(err)
	feed, err := dbh.GetFeedById(feed_id)
	handleError(err)
	rend.JSON(http.StatusOK, FeedJSON{Feed: feed})
}

func addFeed(rend render.Render, req *http.Request, w http.ResponseWriter, dbh *db.DbDispatcher) {

	err := req.ParseForm()
	handleError(err)

	f := &FeedJSON{}
	err = json.NewDecoder(req.Body).Decode(f)
	handleError(err)
	if f.Feed == nil {
		rend.JSON(http.StatusBadRequest, "Malformed request, no Feed found.")
		return
	}

	_, err = dbh.GetFeedByUrl(f.Feed.Url)
	if err == nil {
		rend.JSON(
			http.StatusConflict,
			fmt.Sprintf("A feed already exists with url %s", f.Feed.Url),
		)
		return
	}

	feed, err := dbh.AddFeed(f.Feed.Name, f.Feed.Url)
	handleError(err)
	w.Header().Set("Location", fmt.Sprintf("/feeds/%d", feed.Id))
	rend.JSON(http.StatusCreated, feed)
}

func deleteFeed(params martini.Params, dbh *db.DbDispatcher) int {
	feed_id, err := strconv.Atoi(params["id"])
	handleError(err)

	feed, err := dbh.GetFeedById(feed_id)
	handleError(err)

	dbh.RemoveFeed(feed.Url, true)

	return http.StatusNoContent
}

// TODO: complete this.
func updateFeed() int {
	return http.StatusOK
}
