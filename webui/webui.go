package webui

import (
	"fmt"
	"github.com/codegangsta/martini"
	"github.com/golang/glog"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/feed_watcher"
	"net/http"
	"sort"
	"time"
)

func RunWebUi(config *config.Config, feeds map[string]*feed_watcher.FeedWatcher) {
	m := martini.Classic()
	m.Get("/feedz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello: I have %v feeds.\n", len(feeds))
		mk := make([]string, len(feeds))
		i := 0
		for k, _ := range feeds {
			mk[i] = k
			i++
		}
		sort.Strings(mk)

		for _, uri := range mk {
			f := (feeds)[uri]
			fmt.Fprintf(w, "Feed (%s)-%s\n", f.FeedInfo.Name, uri)
			fmt.Fprintf(w, "  Polling? %t\n", f.Polling())
			fmt.Fprintf(w, "  Crawling? %t\n", f.Crawling())
			fmt.Fprintf(w, "  Known GUIDS: %d\n", len(f.KnownGuids))
			fmt.Fprintf(w, "  Last Crawl Status (%s):\n",
				f.FeedInfo.LastPollTime.Local().Format(time.RFC1123))
			fmt.Fprintf(w, "    HTTP Response: %s\n", f.LastCrawlResponse.HttpResponseStatus)
			if f.FeedInfo.LastPollError != "" {
				fmt.Fprintf(w, "    Error: %s\n", f.FeedInfo.LastPollError)
			}
			fmt.Fprint(w, "\n")
		}
	})
	glog.Fatal(http.ListenAndServe(config.WebServer.ListenAddress, m))
}
