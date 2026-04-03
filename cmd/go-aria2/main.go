package main

import (
	"log"
	"os"

	"github.com/chenjia404/go-aria2/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}
