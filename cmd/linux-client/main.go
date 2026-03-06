package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// time to wait after failed announcement
const retrySleep = time.Second

func main() {
	tracker := flag.String("tracker", "127.0.0.1:4917", "ip:port of the tracker")
	rpcPort := flag.Int("port", 1984, "port to run the RPC server on")
	rpcCommand := flag.String("cmd", "rpc-server", "command to run the RPC server")
	flag.Parse()

	args := []string{
		"--port", fmt.Sprint(*rpcPort),
	}
	args = append(args, flag.Args()...)

	// start RPC server
	cmd := exec.Command(*rpcCommand, args...)
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		panic(err)
	}

	// print command
	log.Printf("Running command: %s %v\n", *rpcCommand, args)

	// start announcement loop
	go func() {
		announceUrl := fmt.Sprintf("http://%s/announce?port=%d", *tracker, *rpcPort)

		for {
			// send announce request
			resp, err := http.Post(announceUrl, "", nil)
			if err != nil {
				log.Printf("Failed to announce to tracker: %v\n", err)
				time.Sleep(retrySleep)
				continue
			}

			// parse response
			var response announcementResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				log.Printf("Failed to parse announcement response: %v\n", err)
				time.Sleep(retrySleep)
				continue
			}

			log.Printf("Announced to server, reannouncing in %v seconds\n", response.Interval)

			// wait for next announcement time
			time.Sleep(time.Duration(response.Interval * float64(time.Second)))
		}
	}()

	cmd.Wait()
}

type announcementResponse struct {
	Interval float64 `json:"interval"`
}
