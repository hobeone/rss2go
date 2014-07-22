package httpclient

import (
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

var starter sync.Once
var addr net.Addr

func testHandler(w http.ResponseWriter, req *http.Request) {
	time.Sleep(500 * time.Millisecond)
	io.WriteString(w, "hello, world!\n")
}

func testDelayedHandler(w http.ResponseWriter, req *http.Request) {
	time.Sleep(2100 * time.Millisecond)
	io.WriteString(w, "hello, world ... in a bit\n")
}

func setupMockServer(t *testing.T) {
	http.HandleFunc("/test", testHandler)
	http.HandleFunc("/test-delayed", testDelayedHandler)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen - %s", err.Error())
	}
	go func() {
		err = http.Serve(ln, nil)
		if err != nil {
			t.Fatalf("failed to start HTTP server - %s", err.Error())
		}
	}()
	addr = ln.Addr()
}

func TestDefaultConfig(t *testing.T) {
	starter.Do(func() { setupMockServer(t) })

	httpClient := NewTimeoutClient()
	req, _ := http.NewRequest("GET", "http://"+addr.String()+"/test-delayed", nil)

	httpClient = NewTimeoutClient()

	_, err := httpClient.Do(req)
	if err == nil {
		t.Fatalf("request should have timed out")
	}

}

func TestHttpClient(t *testing.T) {
	starter.Do(func() { setupMockServer(t) })

	httpClient := NewTimeoutClient()

	req, _ := http.NewRequest("GET", "http://"+addr.String()+"/test", nil)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("1st request failed - %s", err.Error())
	}
	defer resp.Body.Close()

	connectTimeout := (250 * time.Millisecond)
	readWriteTimeout := (50 * time.Millisecond)

	httpClient = NewTimeoutClient(connectTimeout, readWriteTimeout)

	resp, err = httpClient.Do(req)
	if err == nil {
		t.Fatalf("2nd request should have timed out")
	}

	resp, err = httpClient.Do(req)
	if resp != nil {
		t.Fatalf("3nd request should not have timed out")
	}

}
