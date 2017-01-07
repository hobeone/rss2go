package webui

import (
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/labstack/echo"
)

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
