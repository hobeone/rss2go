package db

import "github.com/hobeone/gomigrate"

func migrationsCopy(m []gomigrate.Migration) []*gomigrate.Migration {
	r := make([]*gomigrate.Migration, len(m))
	for i, mig := range m {
		c := mig
		r[i] = &c
	}
	return r
}

// SchemaMigrations gives each caller a new copy of the migrations.  This is
// mostly useful to allow unit tests to run in parallel.
func SchemaMigrations() []*gomigrate.Migration {
	return migrationsCopy(schemaMigrations)
}

// TestFixtures gives the standard fixtures for db testing
func TestFixtures() []*gomigrate.Migration {
	return migrationsCopy(testFixtures)
}

// SchemaMigrations contains the series of migrations needed to create and
// update the rss2go db schema.
var schemaMigrations = []gomigrate.Migration{
	{
		ID:   100,
		Name: "Base Schema",
		Up: `CREATE TABLE "user" (
	"id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
	"name" varchar(255) NOT NULL UNIQUE,
	"email" varchar(255) NOT NULL UNIQUE,
	"enabled" bool NOT NULL,
	"password" text NOT NULL
);
CREATE TABLE "user_feeds" (
	"id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
	"user_id" bigint NOT NULL,
	"feed_info_id" bigint NOT NULL
);
CREATE TABLE "feed_info" (
	"id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
	"name" text NOT NULL UNIQUE,
	"url" text NOT NULL UNIQUE,
	"last_poll_time" datetime NOT NULL,
	"last_poll_error" text NOT NULL
);
CREATE TABLE "feed_item" (
	"id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
	"feed_info_id" bigint NOT NULL,
	"guid" text NOT NULL,
	"added_on" datetime NOT NULL
);
CREATE UNIQUE INDEX user_feed_idx ON user_feeds (user_id,feed_info_id);`,
		Down: `
				"DROP TABLE user",
				"DROP TABLE user_feeds",
				"DROP TABLE feed_item",
				"DROP TABLE feed_info",
				`,
	},
	{
		ID:   110,
		Name: "Add error response",
		Up:   `ALTER TABLE "feed_info" ADD COLUMN last_error_response text;`,
	},
	{
		ID:   120,
		Name: "Add Feed Site link",
		Up:   `ALTER TABLE "feed_info" ADD COLUMN site_url text;`,
	},
	{
		ID:   130,
		Name: "Add Unique constraint to feed_item",
		Up:   `CREATE UNIQUE INDEX guid_feed_unique_index ON feed_item (feed_info_id,guid);`,
	},
}

// TestFixtures contains the base fixture data for testing with a db.
var testFixtures = []gomigrate.Migration{
	{
		ID:   900,
		Name: "Base Test Fixtures",
		Up: `INSERT INTO "user" VALUES(1,'testuser1','test1@example.com',1,'pass1');
			INSERT INTO "user" VALUES(2,'testuser2','test2@example.com',1,'pass2');
			INSERT INTO "user" VALUES(3,'testuser3','test3@example.com',1,'pass3');

			INSERT INTO "feed_info" (id, name, url, last_poll_time, last_poll_error, site_url) VALUES(1,'testfeed1','http://localhost/feed1.atom', '0001-01-01 00:00:00+00:00','','');
			INSERT INTO "feed_info" (id, name, url, last_poll_time, last_poll_error, site_url) VALUES(2,'testfeed2','http://localhost/feed2.atom','0001-01-01 00:00:00+00:00','','');
			INSERT INTO "feed_info" (id, name, url, last_poll_time, last_poll_error, site_url) VALUES(3,'testfeed3','http://localhost/feed3.atom','0001-01-01 00:00:00+00:00','','');

			INSERT INTO "user_feeds" VALUES(1,1,1);
			INSERT INTO "user_feeds" VALUES(2,1,2);
			INSERT INTO "user_feeds" VALUES(3,1,3);
			INSERT INTO "user_feeds" VALUES(4,2,1);
			INSERT INTO "user_feeds" VALUES(5,2,2);
			INSERT INTO "user_feeds" VALUES(6,2,3);
			INSERT INTO "user_feeds" VALUES(7,3,1);
			INSERT INTO "user_feeds" VALUES(8,3,2);
			INSERT INTO "user_feeds" VALUES(9,3,3);

			DELETE FROM sqlite_sequence;
			INSERT INTO "sqlite_sequence" VALUES('feed_info',3);
			INSERT INTO "sqlite_sequence" VALUES('user',3);
			INSERT INTO "sqlite_sequence" VALUES('user_feeds',9);
			`,
		Down: "",
	},
}
