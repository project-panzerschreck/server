package main

import (
	"log"
	"os"
	"strings"
	"time"
)

func setDefaults(args *args) {

	// set default values for unspecified flags
	if args.Host == nil {
		host := "127.0.0.1"
		args.Host = &host
	}

	if args.Port == nil {
		port := 4917
		args.Port = &port
	}

	if args.IdleTimeout == nil {
		timeout := time.Duration(0)
		args.IdleTimeout = &timeout
	}

	if args.Ramalama == nil {
		if env := os.Getenv("LLAMA_COMMAND"); env != "" {
			args.Ramalama = strings.Split(env, " ")
			if len(args.Ramalama) == 0 {
				log.Fatalln("LLAMA_COMMAND environment variable should not be all whitespace")
			}
		} else {
			args.Ramalama = []string{"llama-server"}
		}
	}
}
