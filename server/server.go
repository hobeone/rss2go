package server

import (
	"fmt"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/feed_watcher"
	"log"
	"net/http"
	_ "net/http/pprof"
	"sort"
	"time"
)

func StartHttpServer(config *config.Config, feeds map[string]*feed_watcher.FeedWatcher) {
	http.HandleFunc("/feedz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello: I have %v feeds.\n", len(feeds))
		mk := make([]string, len(feeds))
		i := 0
		for k, _ := range feeds {
			mk[i] = k
			i++
		}
		sort.Strings(mk)

		for _, uri := range mk {
			f := feeds[uri]
			fmt.Fprintf(w, "Feed (%s)-%s polling? %t. crawling? %t\n",
				f.FeedInfo.Name, uri, f.Polling(), f.Crawling())
			fmt.Fprintf(w, "  Known GUIDS: %d\n", len(f.KnownGuids))
			fmt.Fprintf(w, "  Last Crawl Status (%s):\n",
				f.FeedInfo.LastPollTime.Local().Format(time.RFC1123))
			fmt.Fprintf(w, "    HTTP Response: %s\n", f.LastCrawlResponse.HttpResponseStatus)
			if f.FeedInfo.LastPollError != "" {
				fmt.Fprintf(w, "    Error: %s\n", f.FeedInfo.LastPollError)
			}
			fmt.Fprint(w,"\n")
		}
	})

	log.Printf("Starting http server on %v", config.WebServer.ListenAddress)
	log.Fatal(http.ListenAndServe(config.WebServer.ListenAddress, nil))
}
