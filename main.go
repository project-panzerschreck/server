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

	"github.com/wk-y/rama-swap/llama"
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

	//defaults (according to android app)
	if args.TrackerHost == nil {
		host := "0.0.0.0"
		args.TrackerHost = &host
	}

	if args.TrackerPort == nil {
		port := 50055
		args.TrackerPort = &port
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

	ramalama := llama.Llama{
		Command: args.Ramalama,
	}
	tracker := tracker.NewTracker()
	tracker.AddRoutes(mux)
	scheduler := scheduler.NewFcfsScheduler(ramalama, 49170, *args.IdleTimeout, tracker)
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

	log.Printf("Device tracker listening on http://%s:%d\n", *args.TrackerHost, *args.TrackerPort)
	tl, err := net.Listen("tcp", fmt.Sprintf("%s:%d", *args.TrackerHost, *args.TrackerPort))
	if err != nil {
		log.Fatalf("Failed to listen on tracker port: %v", err)
	}
	defer tl.Close()

	server.HandleHttp(mux)

	go func() {
		err := http.Serve(tl, mux)
		if err != nil {
			log.Fatalf("Failed to serve tracker: %v", err)
		}
	}()

	err = http.Serve(l, mux)

	log.Fatalf("Failed to serve: %v", err)
}
