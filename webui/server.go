package webui

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
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
	//	e.Use(middleware.Recover())
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
	r.GET("/feeds", s.getFeeds)
	r.POST("/feeds", s.addFeed)
	r.GET("/feeds/:id", s.getFeed)
	r.PATCH("/feeds/:id", s.updateFeed)
	r.PUT("/feeds/:id/subscribe", s.subFeed)
	r.PUT("/feeds/:id/unsubscribe", s.getFeed)

	/*
		funcmap := template.FuncMap{
			"routeuri": e.URI,
		}
			t := &Template{
				templates: template.Must(template.New("rss2go").Funcs(funcmap).ParseGlob("public/*.html")),
			}
			e.Renderer = t
	*/
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

func (s *APIServer) subFeed(c echo.Context) error {
	id := c.Param("id")
	feedID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("Unknown feed id: %s", err))
	}

	feed, err := s.DBH.GetFeedByID(feedID)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}

	usertoken := c.Get("user").(*jwt.Token)
	claims := usertoken.Claims.(jwt.MapClaims)
	useremail := claims["name"].(string)

	user, err := s.DBH.GetUserByEmail(useremail)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}

	err = s.DBH.AddFeedsToUser(user, []*db.FeedInfo{feed})
	if err != nil {
		fmt.Println(err)
		return c.String(http.StatusInternalServerError, err.Error())
	}

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return jsonapi.MarshalOnePayload(c.Response(), feed)
}

func (s *APIServer) unsubFeed(c echo.Context) error {
	id := c.Param("id")
	feedID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("Unknown feed id: %s", err))
	}

	feed, err := s.DBH.GetFeedByID(feedID)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}

	usertoken := c.Get("user").(*jwt.Token)
	claims := usertoken.Claims.(jwt.MapClaims)
	useremail := claims["name"].(string)

	user, err := s.DBH.GetUserByEmail(useremail)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}

	err = s.DBH.RemoveFeedsFromUser(user, []*db.FeedInfo{feed})
	if err != nil {
		fmt.Println(err)
		return c.String(http.StatusInternalServerError, err.Error())
	}

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return jsonapi.MarshalOnePayload(c.Response(), feed)
}

func (s *APIServer) updateFeed(c echo.Context) error {
	b := new(db.FeedInfo)

	errors := &apiErrors{}

	err := jsonapi.UnmarshalPayload(c.Request().Body, b)

	if err != nil {
		errors.addError("Bad Input", err, http.StatusBadRequest)
		return c.JSON(http.StatusBadRequest, errors)
	}
	err = s.DBH.SaveFeed(b)
	if err != nil {
		errors.addError("Error saving Feed", err, http.StatusUnprocessableEntity)
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)

		return c.JSONPretty(http.StatusUnprocessableEntity, errors, "  ")
	}
	return c.NoContent(http.StatusNoContent)
}

type apiError struct {
	Title  string            `json:"title"`
	Detail string            `json:"detail,omitempty"`
	Source map[string]string `json:"source,omitempty"`
	Status int               `json:"status,omitempty"`
}

type apiErrors struct {
	Errors []*apiError `json:"errors"`
}

func (e *apiErrors) addError(title string, err error, status int) {
	e.Errors = append(e.Errors, &apiError{
		Title:  title,
		Detail: err.Error(),
		Source: map[string]string{
			"pointer": "data",
		},
		Status: status,
	})
}

type feedsReqBinder struct {
	Name string `json:"name" form:"name" query:"name"`
}

func (s *APIServer) getFeeds(c echo.Context) error {
	b := new(feedsReqBinder)

	if err := c.Bind(b); err != nil {
		return err
	}

	var feeds []*db.FeedInfo
	var err error
	if b.Name != "" {
		feeds, err = s.DBH.GetFeedsByName(b.Name)
	} else {
		feeds, err = s.DBH.GetAllFeeds()
	}
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}

	feedInterface := make([]interface{}, len(feeds))

	for i, f := range feeds {
		feedInterface[i] = f
	}

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return jsonapi.MarshalManyPayload(c.Response(), feedInterface)
}

func (s *APIServer) getFeed(c echo.Context) error {
	id := c.Param("id")
	feedID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("Unknown feed id: %s", err))
	}

	feed, err := s.DBH.GetFeedByID(feedID)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return jsonapi.MarshalOnePayload(c.Response(), feed)
}

func (s *APIServer) addFeed(c echo.Context) error {
	usertoken := c.Get("user").(*jwt.Token)
	claims := usertoken.Claims.(jwt.MapClaims)
	useremail := claims["name"].(string)

	dbuser, err := s.DBH.GetUserByEmail(useremail)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}

	feed := new(db.FeedInfo)

	errors := &apiErrors{}

	err = jsonapi.UnmarshalPayload(c.Request().Body, feed)

	if err != nil {
		errors.addError("Bad Input", err, http.StatusBadRequest)
		return c.JSON(http.StatusBadRequest, errors)
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
			errors.addError("Bad Input", err, http.StatusBadRequest)
			return c.JSON(http.StatusBadRequest, errors)
		}
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
		c.Response().WriteHeader(http.StatusOK)
		return jsonapi.MarshalOnePayload(c.Response(), dbfeed)
	}

	err = s.DBH.SaveFeed(feed)
	if err != nil {
		errors.addError("Error saving Feed", err, http.StatusUnprocessableEntity)
		return c.JSONPretty(http.StatusUnprocessableEntity, errors, "  ")
	}
	err = s.DBH.AddFeedsToUser(dbuser, []*db.FeedInfo{feed})
	if err != nil {
		errors.addError("Error subscribing to Feed", err, http.StatusUnprocessableEntity)
		return c.JSONPretty(http.StatusUnprocessableEntity, errors, "  ")
	}
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return jsonapi.MarshalOnePayload(c.Response(), dbfeed)
}

// AuthInfo decodes auth requests
type AuthInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Expects username & password to be passed as JSON in the POST body
// This is how Ember does it.
func (s *APIServer) login(c echo.Context) error {
	a := new(AuthInfo)

	if err := c.Bind(a); err != nil {
		return err
	}

	dbuser, err := s.DBH.GetUserByEmail(a.Username)
	if err == nil {
		//if bcrypt.CompareHashAndPassword([]byte(dbuser.Password), []byte(a.Password)) == nil {
		token := jwt.New(jwt.SigningMethodHS256)

		// Set claims
		claims := token.Claims.(jwt.MapClaims)
		claims["name"] = dbuser.Email
		claims["admin"] = false
		claims["exp"] = time.Now().Add(time.Hour * 72).Unix()

		// Generate encoded token and send it as response.
		t, err := token.SignedString([]byte("secret"))
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, map[string]string{
			"token": t,
		})
		//}
	}

	logrus.Infof("Unknown user or bad password for: %s", a.Username)
	return c.String(http.StatusUnauthorized, "Bad username or password")
}
