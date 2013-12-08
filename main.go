package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	log.SetFlags(0)

	args := os.Args
	if len(args) <= 1 {
		log.Fatalln("Expected `mode`")
	}
	mode := args[1]

	var err error

	switch mode {
	case "deploy":
		if len(args) <= 2 {
			log.Fatalln("Expected `env`")
		}
		wd, _ := os.Getwd()
		env := args[2]
		err = Deploy(wd, env)
	default:
		err = fmt.Errorf("Unknown mode `%v`", mode)
	}

	if err != nil {
		log.Fatalln(err)
	}
}
