package followermaze

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"

	"golang.org/x/net/context"
)

type eventType uint

const (
	follow eventType = 1 << iota
	unfollow
	broadcast
	privageMsg
	statusUpdate
)

type event struct {
	SequenceID int
	Kind       eventType
	FromID     int
	ToID       int
	RawMessage string
}

type clientConn struct {
	ID int
	net.Conn
}

// StartServer starts a followermaze server listening for events at srcAddr and clients at clientAddr
// firstSequenceID must be set to the first event id to expect (usually 0)
// TODO: implement context?
func StartServer(ctx context.Context, srcAddr, clientAddr string, firstSequenceID int) error {

	// start notifier
	ech, connch, err := notifier(firstSequenceID)
	if err != nil {
		return err
	}

	// start src, client servers
	srcSrv, err := net.Listen("tcp", srcAddr)
	if err != nil {
		return err
	}
	clientSrv, err := net.Listen("tcp", clientAddr)
	if err != nil {
		return err
	}

	// pass the servers and connHandler functions to runListener
	go runListener(clientSrv, clientHandler(connch))
	go runListener(srcSrv, srcHandler(ech))
	return nil

}

func runListener(l net.Listener, h connHandler) {
	for {
		// wait for a connection.
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		// handle the connection in a new goroutine.
		go h(conn)
	}

}

type connHandler func(c net.Conn)

// srcHandler returns a connHandler function that parses an event message
// and sends the event on ech
func srcHandler(ech chan<- event) connHandler {
	return func(c net.Conn) {
		scanner := bufio.NewScanner(c)
		var err error
		for scanner.Scan() {
			e, err := parseEvent(scanner.Text())
			if err != nil {
				log.Printf("srchandler: %v", err)
			}
			ech <- e
		}

		err = scanner.Err()
		if err != nil {
			log.Printf("srchandler: %v", err)
		}

	}

}

// clientHandler returns a connHandler function that parses a client connection message
// and sends the connection on connch
func clientHandler(connch chan<- clientConn) connHandler {
	return func(c net.Conn) {

		scanner := bufio.NewScanner(c)
		var err error
		scanner.Scan()
		if scanner.Err() != nil {
			log.Println(err)
		}
		id, err := strconv.Atoi(scanner.Text())
		if err != nil {
			log.Printf("clienthandler: %v", err)
		}
		connch <- clientConn{id, c}
	}

}

func notifier(lastSequenceID int) (chan<- event, chan<- clientConn, error) {
	ech := make(chan event)
	connch := make(chan clientConn)
	followers := make(map[int][]int)
	conns := make(map[int]clientConn)
	eventQ := make(map[int]event)
	go func() {
		for {

			select {
			case e := <-ech:
				eventQ[e.SequenceID] = e
				// handle events in order
				for {
					if e, ok := eventQ[lastSequenceID]; ok {
						delete(eventQ, e.SequenceID)
						if err := handleEvent(e, followers, conns); err != nil {
							log.Printf("notification error: %v", err)
						}
						lastSequenceID++

					} else {
						break
					}
				}

			case c := <-connch:
				log.Printf("client %d added", c.ID)
				conns[c.ID] = c
			}
		}

	}()
	return ech, connch, nil

}

func handleEvent(e event, followers map[int][]int, conns map[int]clientConn) error {
	switch e.Kind {
	case broadcast:
		var n []int
		log.Printf("broadcast from %d to %v", e.FromID, n)
		for _, c := range conns {
			if err := notifyClient(e, c); err != nil {
				return err
			}
		}
	case privageMsg:
		if c, ok := conns[e.ToID]; ok {
			log.Printf("private from %d to %d", e.FromID, e.ToID)
			return notifyClient(e, c)
		}
	case follow:
		f := followers[e.ToID]
		followers[e.ToID] = append(f, e.FromID) // add follower
		log.Printf("follow from %d to %d, followers %v", e.FromID, e.ToID, followers[e.ToID])
		if c, ok := conns[e.ToID]; ok {
			return notifyClient(e, c)
		}
	case statusUpdate:
		for _, f := range followers[e.FromID] {
			if c, ok := conns[f]; ok {
				if err := notifyClient(e, c); err != nil {
					return err
				}
				log.Printf("status from %d to %v", e.FromID, followers[e.FromID])
			}
		}
	case unfollow:
		// remove follower
		delete(followers, e.ToID)
		log.Printf("%d unfollowed %d", e.FromID, e.ToID)
	default:
		return fmt.Errorf("unknown event type %v", e)
	}
	return nil
}

func notifyClient(e event, c clientConn) error {
	log.Printf("notifying %d of %d from %d", c.ID, e.SequenceID, e.FromID)
	_, err := io.Copy(c.Conn, strings.NewReader(e.RawMessage))
	return err
}

// how to handle closed source connection?
// how to handle closed listener connection?
// handle cancellation? context?

// Notifier
// receive connections from ListenerServer
// receive events from SourceServer
// store in map if out of order
// track sequenceID
// select on both conns and events

//TODO:
// cli w/ configurable addrs for servers
// clean up error handling,
// clean up logging
// graceful stopping?

func parseEvent(s string) (event, error) {
	var e event
	var err error
	parseErr := fmt.Errorf("error parsing %s", s)
	ef := strings.Split(s, "|")
	if len(ef) < 2 {
		return e, parseErr
	}
	e.SequenceID, err = strconv.Atoi(ef[0])
	if err != nil {
		return e, err
	}
	e.RawMessage = fmt.Sprintf("%s\n", s)

	// find event kind
	switch ef[1] {
	case "F":
		e.Kind = follow
		goto parsetofrom
	case "B":
		e.Kind = broadcast
		return e, nil
	case "U":
		e.Kind = unfollow
		goto parsetofrom
	case "P":
		e.Kind = privageMsg
		goto parsetofrom
	case "S":
		e.Kind = statusUpdate
		if len(ef) != 3 {
			return e, parseErr
		}
		e.FromID, err = strconv.Atoi(ef[2])
		return e, nil
	default:
		return e, fmt.Errorf("unknown event type: %s", e)
	}

parsetofrom:
	if len(ef) != 4 {
		return e, parseErr
	}
	e.FromID, err = strconv.Atoi(ef[2])
	e.ToID, err = strconv.Atoi(ef[3])

	return e, err
}
