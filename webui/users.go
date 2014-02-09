package webui

import (
	"encoding/json"
	"fmt"
	"github.com/codegangsta/martini"
	"github.com/hobeone/rss2go/db"
	"github.com/martini-contrib/render"
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

func getUser(rend render.Render, params martini.Params, dbh *db.DbDispatcher) {
	user_id, err := strconv.Atoi(params["id"])
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	u, err := dbh.GetUserById(user_id)
	if err != nil {
		rend.JSON(404, err.Error())
		return
	}

	feeds, err := dbh.GetUsersFeeds(u)
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	feed_ids := make([]int, len(feeds))
	for i, f := range feeds {
		feed_ids[i] = f.Id
	}
	rend.JSON(http.StatusOK, UserJSON{
		User: userWithFeeds{
			*u,
			feed_ids,
		},
	})
}

func getUsers(rend render.Render, req *http.Request, dbh *db.DbDispatcher) {
	err := req.ParseForm()
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	var users []db.User
	param_ids := req.Form["ids[]"]
	if len(param_ids) > 0 {
		user_ids, err := parseParamIds(param_ids)
		if err != nil {
			rend.JSON(500, err.Error())
			return
		}

		users = make([]db.User, len(user_ids))
		for i, uid := range user_ids {
			u, err := dbh.GetUserById(uid)
			if err != nil {
				rend.JSON(404, err.Error())
				return
			}

			users[i] = *u
		}
	} else {
		users, err = dbh.GetAllUsers()
		if err != nil {
			rend.JSON(500, err.Error())
			return
		}

	}

	users_json := make([]userWithFeeds, len(users))
	for i, u := range users {
		feeds, err := dbh.GetUsersFeeds(&u)
		if err != nil {
			rend.JSON(500, err.Error())
			return
		}

		feed_ids := make([]int, len(feeds))
		for i, f := range feeds {
			feed_ids[i] = f.Id
		}

		users_json[i] = userWithFeeds{u, feed_ids}
	}

	rend.JSON(http.StatusOK, UsersJSON{Users: users_json})
	return
}

type unmarshalUserJSON struct {
	Id       int    `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Enabled  *bool  `json:"enabled"`
	Password string `json:"password"`
	Feeds    []int  `json:"feeds"`
}
type unmarshalUserJSONContainer struct {
	User unmarshalUserJSON `json:"user"`
}

func updateUser(rend render.Render, req *http.Request, dbh *db.DbDispatcher, params martini.Params) {
	user_id, err := strconv.Atoi(params["id"])
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	err = req.ParseForm()
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	u := unmarshalUserJSONContainer{}
	u.User.Enabled = nil
	err = json.NewDecoder(req.Body).Decode(&u)
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	dbuser, err := dbh.GetUserById(user_id)
	if err != nil {
		rend.JSON(404, err.Error())
		return
	}

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
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	u := unmarshalUserJSONContainer{}

	err = json.NewDecoder(req.Body).Decode(&u)
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	db_user, err := dbh.AddUser(u.User.Name, u.User.Email, u.User.Password)
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/users/%d", db_user.Id))
	rend.JSON(http.StatusCreated, UserJSON{
		User: userWithFeeds{
			*db_user,
			[]int{},
		},
	})
}

func deleteUser(rend render.Render, params martini.Params, dbh *db.DbDispatcher) {
	user_id, err := strconv.Atoi(params["id"])
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	user, err := dbh.GetUserById(user_id)
	if err != nil {
		rend.JSON(500, err.Error())
		return
	}

	dbh.RemoveUser(user)

	rend.JSON(http.StatusNoContent, "")
}
