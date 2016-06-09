CREATE TABLE "user" (
  "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
  "name" varchar(255) NOT NULL UNIQUE,
  "email" varchar(255) NOT NULL UNIQUE,
  "enabled" bool NOT NULL,
  "password" text NOT NULL
);
CREATE TABLE "user_feed" (
  "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
  "user_id" bigint NOT NULL,
  "feed_id" bigint NOT NULL
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
CREATE TABLE "user_feeds" (
  "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
  "user_id" bigint NOT NULL,
  "feed_info_id" bigint NOT NULL
);
CREATE UNIQUE INDEX user_feed_idx ON user_feeds (user_id,feed_info_id);
