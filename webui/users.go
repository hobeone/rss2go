package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-martini/martini"
	"github.com/hobeone/rss2go/db"
	"github.com/martini-contrib/render"
)

type userWithFeeds struct {
	db.User
	FeedIds []int64 `json:"feeds"`
}

type userJSON struct {
	User userWithFeeds `json:"user"`
}

type usersJSON struct {
	Users []userWithFeeds `json:"users"`
}

func getUser(rend render.Render, params martini.Params, dbh *db.DBHandle) {
	userID, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil {
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}

	u, err := dbh.GetUserByID(userID)
	if err != nil {
		rend.JSON(http.StatusNotFound, err.Error())
		return
	}

	feeds, err := dbh.GetUsersFeeds(u)
	if err != nil {
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}

	feedIDs := make([]int64, len(feeds))
	for i, f := range feeds {
		feedIDs[i] = f.ID
	}
	rend.JSON(http.StatusOK, userJSON{
		User: userWithFeeds{
			*u,
			feedIDs,
		},
	})
}

func getUsers(rend render.Render, req *http.Request, dbh *db.DBHandle) {
	err := req.ParseForm()
	if err != nil {
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}

	var users []db.User
	paramIDs := req.Form["ids[]"]
	if len(paramIDs) > 0 {
		userIDs, err := parseParamIds(paramIDs)
		if err != nil {
			rend.JSON(http.StatusInternalServerError, err.Error())
			return
		}

		users = make([]db.User, len(userIDs))
		for i, uid := range userIDs {
			u, err := dbh.GetUserByID(uid)
			if err != nil {
				rend.JSON(http.StatusNotFound, err.Error())
				return
			}

			users[i] = *u
		}
	} else {
		users, err = dbh.GetAllUsers()
		if err != nil {
			rend.JSON(http.StatusInternalServerError, err.Error())
			return
		}

	}

	uJSON := make([]userWithFeeds, len(users))
	for i, u := range users {
		feeds, err := dbh.GetUsersFeeds(&u)
		if err != nil {
			rend.JSON(http.StatusInternalServerError, err.Error())
			return
		}

		feedIDs := make([]int64, len(feeds))
		for i, f := range feeds {
			feedIDs[i] = f.ID
		}

		uJSON[i] = userWithFeeds{u, feedIDs}
	}

	rend.JSON(http.StatusOK, usersJSON{Users: uJSON})
	return
}

type unmarshalUserJSON struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	Enabled  *bool   `json:"enabled"`
	Password string  `json:"password"`
	Feeds    []int64 `json:"feeds"`
}
type unmarshalUserJSONContainer struct {
	User unmarshalUserJSON `json:"user"`
}

func updateUser(rend render.Render, req *http.Request, dbh *db.DBHandle, params martini.Params) {
	userID, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil {
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}

	err = req.ParseForm()
	if err != nil {
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}

	u := unmarshalUserJSONContainer{}
	u.User.Enabled = nil
	err = json.NewDecoder(req.Body).Decode(&u)
	if err != nil {
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}

	dbuser, err := dbh.GetUserByID(userID)
	if err != nil {
		rend.JSON(http.StatusNotFound, err.Error())
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
	err = dbh.SaveUser(dbuser)
	if err != nil {
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}
	err = dbh.UpdateUsersFeeds(dbuser, u.User.Feeds)
	if err != nil {
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}

}

func addUser(req *http.Request, w http.ResponseWriter, dbh *db.DBHandle, rend render.Render) {
	err := req.ParseForm()
	if err != nil {
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}

	u := unmarshalUserJSONContainer{}

	err = json.NewDecoder(req.Body).Decode(&u)
	if err != nil {
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}

	dbUser, err := dbh.AddUser(u.User.Name, u.User.Email, u.User.Password)
	if err != nil {
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/users/%d", dbUser.ID))
	rend.JSON(http.StatusCreated, userJSON{
		User: userWithFeeds{
			*dbUser,
			[]int64{},
		},
	})
}

func deleteUser(rend render.Render, params martini.Params, dbh *db.DBHandle) {
	userID, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil {
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}

	user, err := dbh.GetUserByID(userID)
	if err != nil {
		if err != nil {
			rend.JSON(http.StatusInternalServerError, err.Error())
			return
		}
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}

	err = dbh.RemoveUser(user)
	if err != nil {
		rend.JSON(http.StatusInternalServerError, err.Error())
		return
	}

	rend.JSON(http.StatusNoContent, "")
}
