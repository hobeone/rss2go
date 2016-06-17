INSERT INTO "user" VALUES(1,'testuser1','test1@example.com',1,'pass1');
INSERT INTO "user" VALUES(2,'testuser2','test2@example.com',1,'pass2');
INSERT INTO "user" VALUES(3,'testuser3','test3@example.com',1,'pass3');

INSERT INTO "feed_info" VALUES(1,'testfeed1','http://localhost/feed1.atom','0001-01-01 00:00:00+00:00','');
INSERT INTO "feed_info" VALUES(2,'testfeed2','http://localhost/feed2.atom','0001-01-01 00:00:00+00:00','');
INSERT INTO "feed_info" VALUES(3,'testfeed3','http://localhost/feed3.atom','0001-01-01 00:00:00+00:00','');

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
