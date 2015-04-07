package followermaze

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
)

type eventType uint

const (
	follow eventType = 1 << iota
	unfollow
	broadcast
	privateMsg
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

// Server represents a followermaze server
type Server struct {
	done      chan struct{}
	running   bool
	srcSrv    net.Listener
	clientSrv net.Listener
}

// Start starts a followermaze server listening for events at srcAddr and clients at clientAddr
// firstSequenceID must be set to the first event id to expect (usually 1)
func Start(srcAddr, clientAddr string, firstSequenceID int) (*Server, error) {

	done := make(chan struct{}) // used to stop server goroutines

	// start notifier
	ech, connch, err := notifier(firstSequenceID, done)
	if err != nil {
		return nil, err
	}

	// start src, client servers
	srcSrv, err := net.Listen("tcp", srcAddr)
	if err != nil {
		return nil, err
	}
	clientSrv, err := net.Listen("tcp", clientAddr)
	if err != nil {
		return nil, err
	}

	// pass the servers and connHandler functions to runListener
	go runListener(clientSrv, clientHandler(connch), done)
	go runListener(srcSrv, srcHandler(ech), done)
	return &Server{done, true, srcSrv, clientSrv}, nil
}

// Stop shuts down both the client and event servers.
// All client connections are closed.
// Substequent calls to Stop() return an error
func (s *Server) Stop() error {

	if !s.running {
		return fmt.Errorf("server already stopped")
	}
	close(s.done) // signal goroutines to shutdown
	s.running = false
	return nil
}

// conn with error is used to improve error handling on shutdown
type conn struct {
	net.Conn
	err error
}

func runListener(l net.Listener, h connHandler, done chan struct{}) {
	for {
		// wait for a connection in a goroutine
		connCh := make(chan conn)
		go func() {
			c, err := l.Accept()
			connCh <- conn{c, err}
		}()

		select {
		case c := <-connCh:
			if c.err != nil {
				log.Printf("connection error: %v", c.err)
			} else {
				go h(c) // handle the connection in a new goroutine
			}
		case <-done: // close listener and return
			_ = l.Close()
			return
		}
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
				log.Printf("event parse error: %v", err)
			}
			ech <- e
		}
		if scanner.Err() != nil {
			log.Printf("srchandler error: %v", err)
		}

	}

}

// clientHandler returns a connHandler function that parses a client connection message
// and sends the connection on connch
func clientHandler(connch chan<- clientConn) connHandler {
	return func(c net.Conn) {
		scanner := bufio.NewScanner(c)
		scanner.Scan()
		if scanner.Err() != nil {
			log.Printf("clienthandler error: %v", scanner.Err())
		}
		id, err := strconv.Atoi(scanner.Text())
		if err != nil {
			log.Printf("clienthandler error: %v", err)
		}
		connch <- clientConn{id, c}
	}

}

// Notifier:
// receives events from srcServer on event channel
// receives client connections from clientServer on clientConn channel
// store pending events in map if out of order
func notifier(lastSequenceID int, done chan struct{}) (chan<- event, chan<- clientConn, error) {
	eventChan := make(chan event)
	connch := make(chan clientConn)
	followers := make(map[int]map[int]int)
	clientEventChan := make(map[int]chan event)
	eventQ := make(map[int]event)

	go func() {
		for {
			select {
			case e := <-eventChan:
				eventQ[e.SequenceID] = e
				// handle events in order
				for {
					if e, ok := eventQ[lastSequenceID]; ok {
						delete(eventQ, e.SequenceID)
						handleEvent(e, followers, clientEventChan)
						lastSequenceID++

					} else {
						break
					}
				}

			case c := <-connch:
				clientEventChan[c.ID] = clientNotifier(c, done)
			case <-done:
				return
			}
		}

	}()
	return eventChan, connch, nil
}

func clientNotifier(c clientConn, done chan struct{}) chan event {
	// allow event in chan while handling another
	ech := make(chan event, 1)

	var e event
	go func() {
		for {
			select {
			case e = <-ech:
				if err := notifyClient(e, c); err != nil {
					log.Printf("error notifying client %d: %s", c.ID, err)
				}
			case <-done:
				return
			}

		}
	}()

	return ech
}

func handleEvent(e event, followers map[int]map[int]int, conns map[int]chan event) {
	switch e.Kind {
	case broadcast:
		for _, c := range conns {
			c <- e
		}
	case privateMsg:
		if c, ok := conns[e.ToID]; ok {
			c <- e
		}
	case follow:
		f, ok := followers[e.ToID]
		if !ok {
			f = make(map[int]int)
		}
		f[e.FromID] = e.FromID // add follower
		followers[e.ToID] = f
		if c, ok := conns[e.ToID]; ok {
			c <- e
		}
	case statusUpdate:
		fers := followers[e.FromID]
		for _, f := range fers {
			if c, ok := conns[f]; ok {
				c <- e
			}
		}
	case unfollow:
		f := followers[e.ToID]
		delete(f, e.FromID)

	default:
		// should never get here unless Kind type is modified and parseEvent is incorrect
		panic(fmt.Sprintf("unknown event type %v", e.Kind))
	}
}

func notifyClient(e event, c clientConn) error {
	_, err := io.Copy(c.Conn, strings.NewReader(e.RawMessage))
	return err
}

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
		e.Kind = privateMsg
		goto parsetofrom
	case "S":
		e.Kind = statusUpdate
		if len(ef) != 3 {
			return e, parseErr
		}
		e.FromID, err = strconv.Atoi(ef[2])
		return e, nil
	default:
		return e, fmt.Errorf("unknown event type: %s", ef[1])
	}

parsetofrom:
	if len(ef) != 4 {
		return e, parseErr
	}
	e.FromID, err = strconv.Atoi(ef[2])
	e.ToID, err = strconv.Atoi(ef[3])

	return e, err
}
