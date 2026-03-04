package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/activation"

	"github.com/wk-y/rama-swap/ramalama"
	"github.com/wk-y/rama-swap/server"
	"github.com/wk-y/rama-swap/server/scheduler"
	"github.com/wk-y/rama-swap/tracker"
)

const defaultPort = 4917
const defaultHost = "127.0.0.1"

const EX_USAGE = 64

func main() {
	mux := http.NewServeMux()

	args, rest, err := parseArgs(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", os.Args[0], err)
		os.Exit(EX_USAGE)
	}

	if len(rest) > 0 {
		fmt.Fprintf(os.Stderr, "%s: unexpected positional argument %v\n", os.Args[0], rest[0])
		os.Exit(EX_USAGE)
	}

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
		if env := os.Getenv("RAMALAMA_COMMAND"); env != "" {
			args.Ramalama = strings.Split(env, " ")
			if len(args.Ramalama) == 0 {
				log.Fatalln("RAMALAMA_COMMAND environment variable should not be all whitespace")
			}
		} else {
			args.Ramalama = []string{"ramalama"}
		}
	}

	ramalama := ramalama.Ramalama{
		Command: args.Ramalama,
	}
	scheduler := scheduler.NewFcfsScheduler(ramalama, 49170, *args.IdleTimeout)
	tracker := tracker.NewTracker()
	tracker.AddRoutes(mux)
	server := server.NewServer(ramalama, scheduler)

	server.ModelNameMangler = func(s string) string {
		return strings.ReplaceAll(s, "/", "_")
	}

	// serve on all systemd sockets
	listeners, err := activation.Listeners()
	if err != nil {
		log.Fatalf("Failed checking for socket activation: %v", err)
	}

	for i, listener := range listeners {
		log.Printf("Listening on socket activation (%d)", i)
		mux := http.NewServeMux()
		server.HandleHttp(mux)

		go func() {
			defer listener.Close()

			err = http.Serve(listener, mux)

			log.Fatalf("Failed to serve: %v", err)
		}()
	}

	// serve on the configured host/port
	log.Printf("Listening on http://%s:%d\n", *args.Host, *args.Port)

	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", *args.Host, *args.Port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer l.Close()

	server.HandleHttp(mux)
	err = http.Serve(l, mux)

	log.Fatalf("Failed to serve: %v", err)
}
