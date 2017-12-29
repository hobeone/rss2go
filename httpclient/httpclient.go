package httpclient

//Verbatim from: https://gist.github.com/dmichael/5710968

import (
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/sirupsen/logrus"
)

// Config encapsulates the basic settings for the HTTPClient
type Config struct {
	ConnectTimeout   time.Duration
	ReadWriteTimeout time.Duration
}

func timeoutDialer(config *Config) func(net, addr string) (c net.Conn, err error) {
	return func(netw, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(netw, addr, config.ConnectTimeout)
		if err != nil {
			return nil, err
		}
		conn.SetDeadline(time.Now().Add(config.ReadWriteTimeout))
		return conn, nil
	}
}

// NewTimeoutClient returns a new *http.Client with timeout set on connection
// read and write operations.
func NewTimeoutClient(args ...interface{}) *http.Client {
	// Default configuration
	config := &Config{
		ConnectTimeout:   1 * time.Second,
		ReadWriteTimeout: 1 * time.Second,
	}

	// merge the default with user input if there is one
	if len(args) == 1 {
		timeout := args[0].(time.Duration)
		config.ConnectTimeout = timeout
		config.ReadWriteTimeout = timeout
	}

	if len(args) == 2 {
		config.ConnectTimeout = args[0].(time.Duration)
		config.ReadWriteTimeout = args[1].(time.Duration)
	}

	return &http.Client{
		Transport: &http.Transport{
			Dial: timeoutDialer(config),
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Copied from default function
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			lastURL := via[len(via)-1].URL
			logrus.Debugf("GOT REDIRECT FROM %v TO: %v\n", lastURL, req.URL)
			requestDump, err := httputil.DumpRequest(req, true)
			if err != nil {
				logrus.Errorf("Couldn't dump request: %s", err)
			} else {
				logrus.Debugln(string(requestDump))
			}
			return nil
		},
	}
}
