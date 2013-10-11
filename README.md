rss2go
======

Clone of rss2email in Go.

*Basic Architecture:*

Config is stored in toml format.  By default the config lives in ~/.rss2go/config.toml

List of Feeds and their state (what guids we've already seen) are kept in a
SQlite database.


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

./run.sh runone --config_file config.toml --send_mail=false --loops 1 http://localhost/test.rss


To Build a binary:

go build -o rss2go bin/*.go
