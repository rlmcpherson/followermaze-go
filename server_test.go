package followermaze

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestMain(m *testing.M) {
	flag.Parse()
	// log w/ line numbers if verbose
	if testing.Verbose() {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	} else {
		log.SetOutput(ioutil.Discard)
	}
	os.Exit(m.Run())
}

// runs the test jar file
func TestStartServerJar(t *testing.T) {

	if testing.Short() {
		t.Skipf("skipping jar test in short mode")
	}

	// skip if incorrect java version
	vc := exec.Command("java", "-version")
	vc.Env = []string{}

	out, err := vc.CombinedOutput()
	if err != nil {
		t.Skipf("error checking java version, skipping jar test: %v", err)
	}
	if !strings.Contains(string(out), "1.7.0") {
		t.Skipf("incompatible jvm version: %s", string(out))
	}

	s, err := Start("localhost:9090", "localhost:9099", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	c := exec.Command("time", "java", "-server", "-Xmx1G", "-jar", "./follower-maze-2.0.jar")
	// set event total and randomize seed
	// 1 million events executes in ~26 seconds on retina MBP, so used to limit test duration
	c.Env = []string{"totalEvents=1000000", fmt.Sprintf("randomSeed=%d", rand.Intn(1e5))}
	out, err = c.CombinedOutput()
	if err != nil {
		t.Log(string(out))
		t.Errorf("followermaze test error: %v", err)
	}
	t.Log(string(out))
}

func TestBadAddrs(t *testing.T) {
	var addrTests = []struct {
		srcAddr    string
		clientAddr string
		errStr     string
	}{
		{"localhost", "", "missing port"},
		{"", "localhost", "missing port"},
	}

	for _, tt := range addrTests {
		s, err := Start(tt.srcAddr, tt.clientAddr, 1)
		if err != nil {
			if !strings.Contains(err.Error(), tt.errStr) {
				t.Error(err)
			}
		} else {
			_ = s.Stop()
		}

	}
}

func TestParseEvent(t *testing.T) {
	var evTests = []struct {
		in  string
		e   event
		err bool
	}{
		{"666|F|60|50", event{666, follow, 60, 50, "666|F|60|50\n"}, false},
		{"666|U|60|50", event{666, unfollow, 60, 50, "666|U|60|50\n"}, false},
		{"666|P|60|50", event{666, privateMsg, 60, 50, "666|P|60|50\n"}, false},
		{"666|B", event{666, broadcast, 0, 0, "666|B\n"}, false},
		{"1|S|1", event{1, statusUpdate, 1, 0, "1|S|1\n"}, false},
		{"1|S|60|50", event{}, true},
		{"", event{}, true},
		{"foo|foo", event{}, true},
		{"1|Q|60|50", event{}, true},
		{"666|P|60|50|1", event{}, true},
	}
	for _, tt := range evTests {
		e, err := parseEvent(tt.in)
		if err != nil {
			if tt.err {
				continue
			}
			t.Errorf("unexpected parse error: %v", err)
		}
		if !reflect.DeepEqual(e, tt.e) {
			t.Errorf("expected: %v, got: %v", tt.e, e)
		}
	}
}

func BenchmarkParseEvent(b *testing.B) {
	var evTests = []struct {
		in  string
		e   event
		err bool
	}{
		{"666|F|60|50", event{666, follow, 60, 50, "666|F|60|50\n"}, false},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parseEvent(evTests[0].in)
		if err != nil {
			b.Fatal(err)
		}
	}
}

type testClient struct {
	clientConn
	evExp []event
	evRec []event
}

func TestStartServer(t *testing.T) {

	// map to randomize event order
	var em = map[int]event{
		1: event{1, follow, 2, 1, "1|F|2|1\n"},
		2: event{2, follow, 1, 2, "2|F|1|2\n"},
		3: event{3, broadcast, 0, 0, "3|B\n"},
		4: event{4, statusUpdate, 2, 0, "4|S|2\n"},
		5: event{5, unfollow, 1, 2, "5|U|1|2\n"},
		6: event{6, statusUpdate, 2, 0, "6|S|2\n"},
		7: event{7, privateMsg, 1, 4, "7|P|1|4\n"},
		8: event{8, broadcast, 0, 0, "|\n"}, // bad event
	}

	var clients = []testClient{
		{clientConn{ID: 1},
			[]event{em[1], em[3], em[4]}, nil},
		{clientConn{ID: 2},
			[]event{em[2], em[3]}, nil},
		{clientConn{ID: 3},
			[]event{em[3]}, nil},
		{clientConn{ID: 4},
			[]event{em[3], em[7]}, nil},
	}
	srcAddr, clientAddr := "localhost:9090", "localhost:9099"

	// start server
	s, err := Start(srcAddr, clientAddr, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	// start clients
	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		startClient(t, c, clientAddr, &wg)
	}

	// send events
	srcConn, err := net.Dial("tcp", srcAddr)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range em {
		_, err := io.WriteString(srcConn, e.RawMessage)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("sent event %d", e.SequenceID)
	}
	wg.Wait() // wait until all events received by all clients

}

func startClient(t *testing.T, c testClient, addr string, wg *sync.WaitGroup) {
	// connect
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("client %d: %v", c.ID, err)
	}
	if _, err := io.WriteString(conn, fmt.Sprintf("%d\n", c.ID)); err != nil {
		t.Fatal(err)
	}
	go func() {
		s := bufio.NewScanner(conn)
		for s.Scan() {
			e, err := parseEvent(s.Text())
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%d received %v", c.ID, e.SequenceID)
			er := append(c.evRec, e)
			c.evRec = er
			if reflect.DeepEqual(c.evExp, c.evRec) {
				t.Logf("%d, received all", c.ID)
				_ = conn.Close()
				wg.Done()
				return
			}

		}
		if s.Err() != nil {
			t.Fatal(err)
		}

	}()
	return
}

func TestStopNoStart(t *testing.T) {
	s := &Server{}
	err := s.Stop()
	if err.Error() != "server already stopped" {
		t.Fatal(err)
	}
}
