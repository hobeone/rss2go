package webui

import (
	"encoding/json"
	"fmt"
	"github.com/codegangsta/martini"
	"github.com/hobeone/martini-contrib/render"
	"github.com/hobeone/rss2go/db"
	"net/http"
	"strconv"
)

type userWithFeeds struct {
	db.User
	FeedIds []int `json:"feeds"`
}

type UserJSON struct {
	User userWithFeeds `json:"user"`
}

type UsersJSON struct {
	Users []userWithFeeds `json:"users"`
}

func getUser(r render.Render, params martini.Params, dbh *db.DbDispatcher) {
	user_id, err := strconv.Atoi(params["id"])
	handleError(err)

	u, err := dbh.GetUserById(user_id)
	handleError(err)

	feeds, err := dbh.GetUsersFeeds(u)
	handleError(err)

	feed_ids := make([]int, len(feeds))
	for i, f := range feeds {
		feed_ids[i] = f.Id
	}
	r.JSON(http.StatusOK, UserJSON{
		User: userWithFeeds{
			*u,
			feed_ids,
		},
	})
}

func getUsers(r render.Render, req *http.Request, dbh *db.DbDispatcher) {
	err := req.ParseForm()
	handleError(err)

	var users []db.User
	param_ids := req.Form["ids[]"]
	if len(param_ids) > 0 {
		user_ids, err := parseParamIds(param_ids)
		handleError(err)
		users = make([]db.User, len(user_ids))
		for i, uid := range user_ids {
			u, err := dbh.GetUserById(uid)
			handleError(err)
			users[i] = *u
		}
	} else {
		users, err = dbh.GetAllUsers()
		handleError(err)
	}

	users_json := make([]userWithFeeds, len(users))
	for i, u := range users {
		feeds, err := dbh.GetUsersFeeds(&u)
		handleError(err)

		feed_ids := make([]int, len(feeds))
		for i, f := range feeds {
			feed_ids[i] = f.Id
		}

		users_json[i] = userWithFeeds{u, feed_ids}
	}

	r.JSON(http.StatusOK, UsersJSON{Users: users_json})
	return
}

type unmarshalUserJSON struct {
	Id      int    `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Enabled *bool  `json:"enabled"`
	Feeds   []int  `json:"feeds`
}
type unmarshalUserJSONContainer struct {
	User unmarshalUserJSON `json:"user"`
}

func updateUser(req *http.Request, dbh *db.DbDispatcher, params martini.Params) {
	user_id, err := strconv.Atoi(params["id"])
	handleError(err)

	err = req.ParseForm()
	handleError(err)

	u := unmarshalUserJSONContainer{}
	u.User.Enabled = nil
	err = json.NewDecoder(req.Body).Decode(&u)
	handleError(err)

	dbuser, err := dbh.GetUserById(user_id)
	handleError(err)
	if u.User.Email != "" {
		dbuser.Email = u.User.Email
	}
	if u.User.Name != "" {
		dbuser.Name = u.User.Name
	}
	if u.User.Enabled != nil {
		dbuser.Enabled = *u.User.Enabled
	}
	dbh.SaveUser(dbuser)
	dbh.UpdateUsersFeeds(dbuser, u.User.Feeds)
}

func addUser(req *http.Request, w http.ResponseWriter, dbh *db.DbDispatcher, rend render.Render) {
	err := req.ParseForm()
	handleError(err)
	u := unmarshalUserJSONContainer{}

	err = json.NewDecoder(req.Body).Decode(&u)
	handleError(err)

	db_user, err := dbh.AddUser(u.User.Name, u.User.Email)
	handleError(err)

	w.Header().Set("Location", fmt.Sprintf("/users/%d", db_user.Id))
	rend.JSON(http.StatusCreated, UserJSON{
		User: userWithFeeds{
			*db_user,
			[]int{},
		},
	})
}

func deleteUser(params martini.Params, dbh *db.DbDispatcher) int {
	user_id, err := strconv.Atoi(params["id"])
	handleError(err)

	user, err := dbh.GetUserById(user_id)
	handleError(err)

	dbh.RemoveUser(user)

	return http.StatusNoContent
}
