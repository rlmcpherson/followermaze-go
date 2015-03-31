package followermaze

import (
	"bufio"
	"fmt"
	"io/ioutil"
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

type listenerConn struct {
	listenerID uint64
	net.Conn
}

func start() {

	// start notifier
	// start src, client servers by passing funcs to runListener

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

func clientHandler(connch chan<- listenerConn) connHandler {
	return func(c net.Conn) {}
}

func notifier() (chan<- Event, chan<- listenerConn, error) {
	ech := make(chan Event)
	connch := make(chan listenerConn)
	return ech, connch, nil

}

func runListener(l net.Listener, handler connHandler) {
	for {
		// Wait for a connection.
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		// Handle the connection in a new goroutine.
		// The loop then returns to accepting, so that
		// multiple connections may be served concurrently.
		go func(c net.Conn) {
			// Echo all incoming data.
			//io.Copy(c, c)
			b, err := ioutil.ReadAll(c)
			if err != nil {
				panic(err)
			}
			log.Println(string(b))

			// Shut down the connection.
			c.Close()
		}(conn)
	}

}

// how to handle closed source connection?
// how to handle closed listener connection?

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
