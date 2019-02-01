package main

import (
	"kubelogs/command"
	"log"
)

func main() {
	cmd := command.NewCmdLogs()
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
