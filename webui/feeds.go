package webui

import (
	"fmt"

	"net/http"
	"strconv"

	"github.com/codegangsta/martini"
	"github.com/martini-contrib/binding"

	"github.com/golang/glog"
	"github.com/hobeone/rss2go/db"
	"github.com/martini-contrib/render"
)

type FeedsJSON struct {
	Feeds []FeedJSONItem `json:"feeds"`
}

type FeedJSONItem struct {
	db.FeedInfo
}

type FeedJSON struct {
	Feed *db.FeedInfo `json:"feed" binding:"required"`
}

func getFeeds(rend render.Render, r *http.Request, params martini.Params, dbh *db.DBHandle) {
	if err := r.ParseForm(); err != nil {
		rend.JSON(500, fmt.Sprintf("Couldn't parse request: %s", err.Error()))
		return
	}
	var feed_json []FeedJSONItem
	if len(r.Form["ids[]"]) > 0 {
		feed_ids, err := parseParamIds(r.Form["ids[]"])
		if err != nil {
			rend.JSON(500, fmt.Errorf("Couldn't parse request: %s", err.Error()))
			return
		}
		feed_json = make([]FeedJSONItem, len(feed_ids))
		for i, feed_id := range feed_ids {
			feed, err := dbh.GetFeedById(feed_id)
			if err != nil {
				rend.JSON(404, err.Error())
				return
			}

			feed_json[i] = FeedJSONItem{*feed}
		}
		glog.Infof("Got %d feeds", len(feed_json))
	} else {
		feeds, err := dbh.GetAllFeeds()
		if err != nil {
			rend.JSON(500, err.Error())
			return
		}

		feed_json = make([]FeedJSONItem, len(feeds))
		for i, feed := range feeds {
			feed_json[i] = FeedJSONItem{feed}
		}
		glog.Infof("Got %d feeds", len(feed_json))
	}
	rend.JSON(http.StatusOK, FeedsJSON{feed_json})
}

func getFeed(rend render.Render, dbh *db.DBHandle, params martini.Params) {
	feed_id, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	feed, err := dbh.GetFeedById(feed_id)
	if err != nil {
		rend.JSON(404, err.Error())
		return
	}

	rend.JSON(http.StatusOK, FeedJSON{Feed: feed})
}

func (f FeedJSON) Validate(errors *binding.Errors, req *http.Request) {
	if f.Feed == nil {
		errors.Add([]string{"Feed"}, "error", "Feed must exist.")
	}
}

func addFeed(rend render.Render, req *http.Request, w http.ResponseWriter, dbh *db.DBHandle, f FeedJSON, ctx martini.Context) {
	_, err := dbh.GetFeedByUrl(f.Feed.Url)
	if err == nil {
		rend.JSON(
			http.StatusConflict,
			fmt.Sprintf("A feed already exists with url %s", f.Feed.Url),
		)
		return
	}

	feed, err := dbh.AddFeed(f.Feed.Name, f.Feed.Url)
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/feeds/%d", feed.Id))
	rend.JSON(http.StatusCreated, FeedJSON{Feed: feed})
}

func deleteFeed(rend render.Render, params martini.Params, dbh *db.DBHandle) {
	feed_id, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	feed, err := dbh.GetFeedById(feed_id)
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	err = dbh.RemoveFeed(feed.Url, true)
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	rend.JSON(http.StatusNoContent, "")
}

func updateFeed(rend render.Render, req *http.Request, dbh *db.DBHandle, params martini.Params, f FeedJSON) {
	feed_id, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	dbfeed, err := dbh.GetFeedById(feed_id)
	if err != nil {
		rend.JSON(404, err.Error())
		return
	}

	dbfeed.Name = f.Feed.Name
	dbfeed.Url = f.Feed.Url
	dbfeed.LastPollTime = f.Feed.LastPollTime

	dbh.SaveFeed(dbfeed)
	rend.JSON(http.StatusOK, FeedJSON{Feed: dbfeed})
}
