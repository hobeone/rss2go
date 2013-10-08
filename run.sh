#!/bin/sh

# go run *.go doesn't like having _test.go files in the glob.

MAINFILES=`ls -1 bin/*.go | grep -v _test.go | xargs`

go run $MAINFILES
