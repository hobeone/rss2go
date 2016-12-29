package webui

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/davecgh/go-spew/spew"
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
	//	e.Use(middleware.Recover())
	e.Use(basicAuth(s.DBH))
	e.Use(addDBH(s.DBH))
	e.Use(middleware.BodyLimit("1024K"))
	e.Use(middleware.Gzip())
	e.Use(headers)

	e.GET("/", s.userOverview)
	e.POST("/unsubscribe/:id", s.unsubscribe)
	e.POST("/subscribe", s.subscribe)

	funcmap := template.FuncMap{
		"routeuri": e.URI,
	}

	t := &Template{
		templates: template.Must(template.New("rss2go").Funcs(funcmap).ParseGlob("public/*.html")),
	}
	e.Renderer = t

	spew.Dump(e.URI(s.unsubscribe, 10))
	customServer := &http.Server{
		Addr:           ":7999",
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

// Template implements the template functionality needed for Echo
type Template struct {
	templates *template.Template
}

// Render implements the echo Render interface
func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

// Set standard headers for all responses
func headers(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		//		c.Response().Header().Set("Content-Type", "application/json; charset=utf-8")
		c.Response().Header().Set("Access-Control-Allow-Origin", "*")
		c.Response().Header().Set("Access-Control-Expose-Headers", "link,content-length")
		c.Response().Header().Set("Strict-Transport-Security", "max-age=86400")
		return next(c)
	}
}
func addDBH(dbh *db.Handle) echo.MiddlewareFunc {
	// Defaults
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("dbh", dbh)
			return next(c)
		}
	}
}

const (
	basic = "Basic"
)

func basicAuth(dbh *db.Handle) echo.MiddlewareFunc {
	// Defaults
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			auth := c.Request().Header.Get(echo.HeaderAuthorization)
			l := len(basic)

			if len(auth) > l+1 && auth[:l] == basic {
				b, err := base64.StdEncoding.DecodeString(auth[l+1:])
				if err != nil {
					return err
				}
				cred := string(b)
				for i := 0; i < len(cred); i++ {
					if cred[i] == ':' {
						// Verify credentials
						dbuser, err := dbh.GetUserByEmail(cred[:i])
						if err == nil {
							//if bcrypt.CompareHashAndPassword([]byte(dbuser.Password), []byte(cred[i+1:])) == nil {
							c.Set("user", dbuser)
							return next(c)
							//}
							//logrus.Infof("Bad password for user: %s", cred[:i])
						}
						logrus.Infof("Unknown user authentication: %s", cred[:i])
					}
				}
			}
			// Need to return `401` for browsers to pop-up login box.
			c.Response().Header().Set(echo.HeaderWWWAuthenticate, basic+" realm=Restricted")
			return echo.ErrUnauthorized
		}
	}
}

type userOverviewData struct {
	Feeds        []db.FeedInfo
	UnSubHandler echo.HandlerFunc
	SubHandler   echo.HandlerFunc
}

func (s *APIServer) userOverview(c echo.Context) error {
	user := c.Get("user").(*db.User)
	feeds, err := s.DBH.GetUsersFeeds(user)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}
	return c.Render(http.StatusOK, "useroverview.html", userOverviewData{
		Feeds:        feeds,
		UnSubHandler: s.unsubscribe,
		SubHandler:   s.subscribe,
	})
}

func (s *APIServer) unsubscribe(c echo.Context) error {
	// Delete the subscripttion and then redir to userOverview
	user := c.Get("user").(*db.User)
	id := c.Param("id")
	feedID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("Unknown feed id: %s", err))
	}
	feed, err := s.DBH.GetFeedByID(feedID)
	if err != nil {
		return c.String(http.StatusNotFound, "Unknown feed id")
	}
	s.DBH.RemoveFeedsFromUser(user, []*db.FeedInfo{feed})
	return c.Redirect(302, c.Echo().URI(s.userOverview))
}

func (s *APIServer) subscribe(c echo.Context) error {
	user := c.Get("user").(*db.User)
	feedURI := c.FormValue("uri")
	feedName := c.FormValue("name")

	feed, err := s.DBH.GetFeedByURL(feedURI)
	if err != nil {
		// Feed doesn't exist
		feed, err = s.DBH.AddFeed(feedName, feedURI)
		if err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Error adding feed: %s", err))
		}
	}
	s.DBH.AddFeedsToUser(user, []*db.FeedInfo{feed})
	return c.Redirect(302, c.Echo().URI(s.userOverview))
}

func (s *APIServer) updateUser(c echo.Context) error {
	return nil
}
