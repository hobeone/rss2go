package webui

import (
	"code.google.com/p/go.crypto/bcrypt"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/codegangsta/martini"
	"github.com/codegangsta/martini-contrib/binding"
	//"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	"github.com/hobeone/martini-contrib/render"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"github.com/hobeone/rss2go/feed_watcher"
	"net/http"
	"strconv"
	"strings"
)

func failAuth(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Basic realm=\"Authorization Required\"")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintln(w, "Not Authorized")
}

var authenticateUser = func (res http.ResponseWriter, req *http.Request, dbh *db.DbDispatcher) {
	auth_header := strings.SplitAfterN(
		strings.TrimSpace(
			req.Header.Get("Authorization"),
		),
		"Basic ",
		2,
	)
	if len(auth_header) > 1 {
		dec_string, err := base64.StdEncoding.DecodeString(auth_header[1])
		if err != nil {
			glog.Errorf("Error decoding string: ", err)
			failAuth(res)
			return
		}
		auth_parts := strings.SplitN(string(dec_string[:]), ":", 2)
		if len(auth_parts) < 2 {
			glog.Errorf("auth string had no ':' in it, failing")
			failAuth(res)
			return
		}
		user_email := auth_parts[0]
		pass := auth_parts[1]
		dbuser, err := dbh.GetUserByEmail(user_email)
		if err != nil {
			glog.Infof("Unknown user authentication: %s", user_email)
			failAuth(res)
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(dbuser.Password), []byte(pass)) != nil {
			failAuth(res)
			return
		}
	} else {
		failAuth(res)
	}
}

func UserAuth() martini.Handler {
	return authenticateUser
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

	m.Use(func(w http.ResponseWriter, req *http.Request) {
		if origin := req.Header.Get("Origin"); origin != "" {
			w.Header().Add("Access-Control-Allow-Origin", origin)
		}
		w.Header().Add("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Add("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token")
		w.Header().Add("Access-Control-Allow-Credentials", "true")
	})

	m.Use(UserAuth())
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
	r.Put("/api/1/feeds/:id", binding.Bind(FeedJSON{}), updateFeed)
	// Ember sends an OPTIONS request before sending a potentially destructive
	// call to see if it will be allowed.
	r.Options("/api/1/feeds/:id", send200)
	// Add Feed
	r.Post("/api/1/feeds", binding.Bind(FeedJSON{}), addFeed)
	r.Delete("/api/1/feeds/:id", deleteFeed)

	// User API
	r.Get("/api/1/users", getUsers)
	r.Get("/api/1/users/:id", getUser)
	r.Options("/api/1/users/:id", send200)
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

func send200() int {
	return http.StatusOK
}

func RunWebUi(config *config.Config, dbh *db.DbDispatcher, feeds map[string]*feed_watcher.FeedWatcher) {
	if config.WebServer.EnableAPI {
		m := createMartini(dbh, feeds)
		glog.Fatal(http.ListenAndServe(config.WebServer.ListenAddress, m))
	}
}
