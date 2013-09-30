package server

import (
	"net/http"
	"sort"
	"fmt"
	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/feed_watcher"
"log"

)

func StartHttpServer(config *config.Config, feeds map[string]*feed_watcher.FeedWatcher) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
			fmt.Fprint(w, "  Last Crawl Status:\n")
			fmt.Fprintf(w, "    HTTP Response: %s\n", f.LastCrawlResponse.HttpResponseStatus)
			fmt.Fprintf(w, "    %#v\n", f.LastCrawlResponse.Error)
		}
	})

	log.Printf("Starting http server on %v", config.WebServer.ListenAddress)
	log.Fatal(http.ListenAndServe(config.WebServer.ListenAddress, nil))
}

