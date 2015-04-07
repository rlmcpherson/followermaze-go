package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"

	"github.com/rlmcpherson/followermaze"
)

var opts = []struct {
	env        string
	defaultVal string
	val        string
}{
	{"SOURCE_ADDR", "localhost:9090", ""},
	{"CLIENT_ADDR", "localhost:9099", ""},
	{"EVENT_SEQUENCE_ID", "1", ""},
}

var sequenceID int

func init() {
	// load opts from environment
	for i, opt := range opts {
		opt.val = os.Getenv(opt.env)
		if opt.val == "" {
			opt.val = opt.defaultVal
		}
		opts[i] = opt
	}
	// parse sequenceID
	var err error
	sequenceID, err = strconv.Atoi(opts[2].val)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	server, err := followermaze.Start(opts[0].val, opts[1].val, sequenceID)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("server started:")
	fmt.Printf("events  %s\n", opts[0].val)
	fmt.Printf("clients %s\n", opts[1].val)

	// stop the server on os interrupt or kill signals
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt, os.Kill)
	s := <-sigch
	fmt.Printf("%s signal, shutting down server...\n", s)
	if err := server.Stop(); err != nil {
		fmt.Printf("error on shutdown: %s\n", err)
		os.Exit(1)
	}
}
