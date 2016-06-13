rss2go
======

[![wercker status](https://app.wercker.com/status/9e619f7630d2c3797ce94f75f654e334 "wercker status")](https://app.wercker.com/project/bykey/9e619f7630d2c3797ce94f75f654e334)

Clone of rss2email in Go that I've used to learn Go.

*Basic Architecture:

Config is stored in json format.  By default the config lives in ~/.config/rss2go/config.json

List of Feeds and their state (what guids we've already seen) are kept in a
SQLite database.


When started up in daemon or runone mode:

Goroutine Pool of HTTP Crawlers
- all listen to channel of FeedCrawlRequests

One FeedWatcher goroutine per Feed
- handles getting the feed, finding new items and sending them out for mailing

One goroutine for the mailer

One gorouting for a HTTP server that exports
/feedz -> shows last poll status of each feed 
/debug/... -> pprof endpoint for debugging

Example usage:
```
go build -o rss2go main.go
mkdir -p ~/.config/rss2go
cp config_example.json ~/.config/rss2go/config.json
```
Edit ~/.config/rss2go/config.json to have the right addresses and paths in it.
```
./rss2go --config ~/.config/rss2go/config.json feeds add "FeedName" 'http://feed/url.atom'
./rss2go --config ~/.config/rss2go/config.json users add yourname your@email password 'http://feed/url.atom'
./rss2go feeds runone --send_mail=false http://localhost/test.rss
```

To Build a binary:
```
go build -o rss2go main.go
```

Upstart config for Ubuntu is in initscripts/upstart/rss2go.conf.  Copy it to
/etc/init/rss2go.conf and install a built binary where it says to have rss2go
run as a service.


REST API
========
Rss2Go can expose a REST API which can be used to edit users and feeds in the system.

To enable it set "enableAPI" to true in the web_server section of the config:

```
[web_server]
listenAddress = "localhost:7000"
enableAPI = false
```

webui/webui.go documents the endpoints.  See https://github.com/hobeone/rss2go_web for a hacky Ember.js based client.

*Note:* There is no authentication whatsoever on the API yet so be careful.
