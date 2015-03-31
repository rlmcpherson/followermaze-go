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

type EventKind uint

const (
	Follow EventKind = 1 << iota
	Unfollow
	Broadcast
	PrivateMsg
	StatusUpdate
)

type Event struct {
	//Payload    string
	SequenceID int
	Kind       EventKind
	FromID     int
	ToID       int
	RawMessage string
}

type clientConn struct {
	ID int
	net.Conn
}

func start() error {

	// start notifier
	ech, connch, err := notifier()
	if err != nil {

		return err
	}

	// start src, client servers by passing funcs to runListener
	srcSrv, err := net.Listen("tcp", "localhost:9090")
	clientSrv, err := net.Listen("tcp", "localhost:9099")
	if err != nil {
		return err
	}
	go runListener(clientSrv, clientHandler(connch))
	runListener(srcSrv, srcHandler(ech))
	// TODO: go this
	return nil

}

type connHandler func(c net.Conn)

func srcHandler(ech chan<- Event) connHandler {
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

//type client struct {
//ID        int
//followers []int
//}

func notifier() (chan<- Event, chan<- clientConn, error) {
	ech := make(chan Event)
	connch := make(chan clientConn)
	followers := make(map[int][]int)
	conns := make(map[int]clientConn)
	eventQ := make(map[int]Event)
	lastSequenceID := 1
	go func() {
		for {

			select {
			case e := <-ech:
				log.Println(e)
				eventQ[e.SequenceID] = e
				for {
					if e, ok := eventQ[lastSequenceID]; ok {
						log.Printf("sending event %d", e.SequenceID)
						err := handleEvent(e, followers, conns)
						if err != nil {
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

func handleEvent(e Event, followers map[int][]int, conns map[int]clientConn) error {
	switch e.Kind {
	case Broadcast:
		var n []int
		log.Printf("broadcast from %d to %v", e.FromID, n)
		for _, c := range conns {
			return notifyClient(e, c)
		}
	case PrivateMsg:
		if c, ok := conns[e.ToID]; ok {
			log.Printf("private from %d to %d", e.FromID, e.ToID)
			return notifyClient(e, c)
		}
	case Follow:
		f := followers[e.ToID]
		followers[e.ToID] = append(f, e.FromID) // add follower
		log.Printf("follow from %d to %d, followers %v", e.FromID, e.ToID, followers[e.ToID])
		if c, ok := conns[e.ToID]; ok {
			return notifyClient(e, c)
		}
	case StatusUpdate:
		for _, f := range followers[e.FromID] {
			if c, ok := conns[f]; ok {
				if err := notifyClient(e, c); err != nil {
					return err
				}
				log.Printf("status from %d to %v", e.FromID, followers[e.FromID])
			}
		}
	case Unfollow:
		// remove follower
		delete(followers, e.ToID)
		log.Printf("%d unfollowed %d", e.FromID, e.ToID)
	default:
		panic("unknown event type")

	}
	return nil
}

func notifyClient(e Event, c clientConn) error {
	log.Printf("notifying %d of %d from %d", c.ID, e.SequenceID, e.FromID)
	_, err := io.Copy(c.Conn, strings.NewReader(e.RawMessage))
	return err
}

func runListener(l net.Listener, h connHandler) {
	for {
		// Wait for a connection.
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		// Handle the connection in a new goroutine.
		go h(conn)
	}

}

// how to handle closed source connection?
// how to handle closed listener connection?
// handle cancellation? context?

// SourceServer
// Parse Message, send to Notifier

// Listener
// Parse message
// send connection to notifier

// Notifier
// receive connections from ListenerServer
// receive events from SourceServer
// store in map if out of order
// track sequenceID
// select on both conns and events

func parseEvent(s string) (Event, error) {
	var e Event
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
		e.Kind = Follow
		goto ParseToFrom
	case "B":
		e.Kind = Broadcast
		return e, nil
	case "U":
		e.Kind = Unfollow
		goto ParseToFrom
	case "P":
		e.Kind = PrivateMsg
		goto ParseToFrom
	case "S":
		e.Kind = StatusUpdate
		if len(ef) != 3 {
			return e, parseErr
		}
		e.FromID, err = strconv.Atoi(ef[2])
		return e, nil
	default:
		return e, fmt.Errorf("unknown event type: %s", e)
	}
ParseToFrom:
	if len(ef) != 4 {
		return e, parseErr
	}
	e.FromID, err = strconv.Atoi(ef[2])
	e.ToID, err = strconv.Atoi(ef[3])

	return e, err
}
