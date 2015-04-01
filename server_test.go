package followermaze

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/net/context"
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

	if err := StartServer(context.Background(), "localhost:9090", "localhost:9099", 1); err != nil {
		t.Fatal(err)
	}
	c := exec.Command("time", "java", "-server", "-Xmx1G", "-jar", "./follower-maze-2.0.jar")
	// limit events to default, otherwise jar apparently runs forever when exec'ed
	// randomize seed
	c.Env = []string{"totalEvents=10000", fmt.Sprintf("randomSeed=%d", rand.Intn(1e5))}
	out, err = c.CombinedOutput()
	if err != nil {
		t.Log(string(out))
		t.Errorf("followermaze test error: %v", err)
	}
	t.Log(string(out))
}

func TestStartServerBadAddrs(t *testing.T) {
	var addrTests = []struct {
		srcAddr    string
		clientAddr string
		errStr     string
	}{
		{"", "", ""},
		{"localhost", "", "missing port"},
		{"", "localhost", "missing port"},
	}

	for _, tt := range addrTests {
		if err := StartServer(context.Background(), tt.srcAddr, tt.clientAddr, 1); err != nil &&
			!strings.Contains(err.Error(), tt.errStr) {
			t.Error(err)
		}

	}
}

func TestParseEvent(t *testing.T) {
	var evTests = []struct {
		in  string
		e   Event
		err bool
	}{
		{"666|F|60|50", Event{666, Follow, 60, 50, "666|F|60|50\n"}, false},
		{"666|U|60|50", Event{666, Unfollow, 60, 50, "666|U|60|50\n"}, false},
		{"666|P|60|50", Event{666, PrivateMsg, 60, 50, "666|P|60|50\n"}, false},
		{"666|B", Event{666, Broadcast, 0, 0, "666|B\n"}, false},
		{"1|S|1", Event{1, StatusUpdate, 1, 0, "1|S|1\n"}, false},
		{"1|S|60|50", Event{}, true},
		{"", Event{}, true},
		{"foo|foo", Event{}, true},
		{"1|Q|60|50", Event{}, true},
		{"666|P|60|50|1", Event{}, true},
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
