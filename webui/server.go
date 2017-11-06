package webui

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/jsonapi"
	"github.com/hobeone/rss2go/db"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
)

// Dependencies contains all of the things the server needs to run
type Dependencies struct {
	DBH *db.Handle
}

// APIServer implements the API serving part of mtgbrew
type APIServer struct {
	Dependencies
	Port int32
}

// Serve sets up and starts the server
func (s *APIServer) Serve() error {
	e := echo.New()
	e.Debug = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())
	e.Use(middleware.BodyLimit("1024K"))
	e.Use(middleware.Gzip())

	// Serve the ember app
	e.File("/", "assets/index.html")
	e.Static("/assets", "assets/assets")

	//	e.OPTIONS("/api/login/", s.updateUser)
	e.POST("/api/login/", s.login)

	// Restricted group
	r := e.Group("/api/v1")
	r.Use(middleware.JWT([]byte("secret")))
	r.Use(s.getDBUser)
	r.GET("/feeds", s.getFeeds)
	r.POST("/feeds", s.addFeed)
	r.GET("/feeds/:id", s.getFeed)
	r.PATCH("/feeds/:id", s.updateFeed)
	r.PUT("/feeds/:id/subscribe", s.subFeed)
	r.PUT("/feeds/:id/unsubscribe", s.unsubFeed)

	customServer := &http.Server{
		Addr:           fmt.Sprintf(":%d", s.Port),
		ReadTimeout:    20 * time.Second,
		WriteTimeout:   20 * time.Second,
		MaxHeaderBytes: 2048,
	}

	err := e.StartServer(customServer)
	if err != nil {
		return fmt.Errorf("Error starting server: %s", err)
	}
	return nil
}

func (s *APIServer) getDBUser(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		usertoken := c.Get("user").(*jwt.Token)
		claims := usertoken.Claims.(jwt.MapClaims)
		useremail := claims["name"].(string)
		dbuser, err := s.DBH.GetUserByEmail(useremail)
		if err != nil {
			return err
		}
		c.Set("dbuser", dbuser)
		return next(c)
	}
}

func (s *APIServer) subFeed(c echo.Context) error {
	feedID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return newError(c, "Unknown feed id", err)
	}

	feed, err := s.DBH.GetFeedByID(feedID)
	if err != nil {
		return newError(c, "Unable to retrieve feed", err)
	}

	dbuser := c.Get("dbuser").(*db.User)

	err = s.DBH.AddFeedsToUser(dbuser, []*db.FeedInfo{feed})
	if err != nil {
		return newError(c, "Unable to subscribe user to feed", err)
	}

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return jsonapi.MarshalPayload(c.Response(), feed)
}

func (s *APIServer) unsubFeed(c echo.Context) error {
	feedID, err := strconv.ParseInt(c.Param("id"), 10, 64)

	if err != nil {
		return newError(c, "Unknown feed", err)
	}

	feed, err := s.DBH.GetFeedByID(feedID)
	if err != nil {
		return newError(c, "Unable to get feed", err)
	}

	dbuser := c.Get("dbuser").(*db.User)
	err = s.DBH.RemoveFeedsFromUser(dbuser, []*db.FeedInfo{feed})
	if err != nil {
		return newError(c, "Unable to unsubscribe user from feed", err)
	}

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return jsonapi.MarshalPayload(c.Response(), feed)
}

func (s *APIServer) updateFeed(c echo.Context) error {
	b := new(db.FeedInfo)
	err := jsonapi.UnmarshalPayload(c.Request().Body, b)

	if err != nil {
		return newError(c, "Bad Input", err)
	}
	err = s.DBH.SaveFeed(b)
	if err != nil {
		return newError(c, "Error saving Feed", err)
	}
	return c.NoContent(http.StatusNoContent)
}

type feedsReqBinder struct {
	Name string `json:"name" form:"name" query:"name"`
}

func (s *APIServer) getFeeds(c echo.Context) error {
	dbuser := c.Get("dbuser").(*db.User)
	b := new(feedsReqBinder)

	if err := c.Bind(b); err != nil {
		return newError(c, "Unable to parse request", err)
	}

	var feeds []*db.FeedInfo
	var err error
	if b.Name != "" {
		feeds, err = s.DBH.GetUsersFeedsByName(dbuser, b.Name)
	} else {
		feeds, err = s.DBH.GetUsersFeeds(dbuser)
	}
	if err != nil {
		return newError(c, "Unable to get feeds", err)
	}

	feedInterface := make([]interface{}, len(feeds))

	for i, f := range feeds {
		feedInterface[i] = f
	}

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return jsonapi.MarshalPayload(c.Response(), feedInterface)
}

func (s *APIServer) getFeed(c echo.Context) error {
	feedID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return newError(c, "Unknown Feed ID", err)
	}

	feed, err := s.DBH.GetFeedByID(feedID)
	if err != nil {
		return newError(c, "Couldn't retrieve Feed information", err)
	}

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return jsonapi.MarshalPayload(c.Response(), feed)
}

func (s *APIServer) addFeed(c echo.Context) error {
	dbuser := c.Get("dbuser").(*db.User)

	feed := new(db.FeedInfo)
	err := jsonapi.UnmarshalPayload(c.Request().Body, feed)
	if err != nil {
		return newError(c, "Bad Input", err)
	}

	//scrub
	//- id
	//- last pool
	//- last error
	feed.ID = 0
	feed.LastPollTime = time.Time{}
	feed.LastErrorResponse = ""
	feed.LastPollError = ""

	// See if Feed already exists:
	// - subscribe
	// Else add & subscribe

	// TODO: normalize the url
	dbfeed, err := s.DBH.GetFeedByURL(feed.URL)
	if err == nil {
		err = s.DBH.AddFeedsToUser(dbuser, []*db.FeedInfo{dbfeed})
		if err != nil {
			return newError(c, "Bad Input", err)
		}
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
		c.Response().WriteHeader(http.StatusOK)
		return jsonapi.MarshalPayload(c.Response(), dbfeed)
	}

	err = s.DBH.SaveFeed(feed)
	if err != nil {
		return newError(c, "Error saving Feed", err)
	}
	err = s.DBH.AddFeedsToUser(dbuser, []*db.FeedInfo{feed})
	if err != nil {
		return newError(c, "Error subscribing to Feed", err)
	}
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return jsonapi.MarshalPayload(c.Response(), dbfeed)
}
