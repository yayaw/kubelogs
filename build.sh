#!/bin/sh

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-w -s' -o download/kubelogs_linux_amd64
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -a -ldflags '-w -s' -o download/kubelogs_windows_amd64
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -a -ldflags '-w -s' -o download/kubelogs_mac_amd64
