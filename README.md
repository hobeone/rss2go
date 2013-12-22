rss2go
======

Clone of rss2email in Go.

*Basic Architecture:*

Config is stored in toml format.  By default the config lives in ~/.rss2go/config.toml

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

go build -o rss2go main.go
mkdir -p ~/.config/rss2go
cp config_example.toml ~/.config/rss2go/config.toml

Edit ~/.config/rss2go/config.toml to have the right addresses and paths in it.

./rss2go addfeed "FeedName" 'http://feed/url.atom'

./rss2go adduser yourname your@email 'http://feed/url.atom'

./rss2go runone --send_mail=false http://localhost/test.rss


To Build a binary:

go build -o rss2go main.go


Upstart config for Ubuntu is in initscripts/upstart/rss2go.conf.  Copy it to
/etc/init/rss2go.conf and install a built binary where it says to have rss2go
run as a service.
