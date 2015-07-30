# followermaze

## Installation

**Requirements:** The followermaze server requires go version >=1.4 and a
working gopath. $GOPATH/bin must be in $PATH to install and run the binary. 

To install and run binary: from the root of the repository, run:

```
cd cmd/followermaze
go install 
followermaze 
```

Alternatively, a runnable binary can be generated from 'cmd/followermaze':

```
go build
./followermaze
```

## Testing

From the root of the git repository, run `go test`. 

To skip the "integration test" that executes `follower-maze-2.0.jar`, run `go
test -short`

The binary (main package) can be tested by running `go test` from the directory
`cmd/followermaze`

Note: other standard go testing tools that were run as part of verification and should run without
errors include:
`go vet`, `golint`, and `errcheck`.

### Test coverage
Test coverage can be measured using the go cover tool and was measured at > 92%
To measure test coverage and view it interactively, run:

```
go test -coverprofile cover.out 
go tool cover -html=cover.out
```
See https://godoc.org/golang.org/x/tools/cmd/cover

Alternatively, view
[testcoverage.html](http://htmlpreview.github.io/?https://github.com/rlmcpherson/followermaze-go/blob/master/testcoverage.html)
(github link) or open it in a web browser.

## Server Options

All options can be set via environment variables:

**SOURCE_ADDR**: sets the address string, including port, for the event
source server. Default: localhost:9090

**CLIENT_ADDR**:  sets the address string, including port, for the client
server. Default: localhost:9099

**EVENT_SEQUENCE_ID**: sets the initial event id to be sent. Default: 1


## Implementation Notes

- **Dependencies** This implementation uses no third party dependencies, only the go standard library. 
- **Logging**: currently only errors are logged since the go standard library
    log package does not support leveled logging. If using third-party
    packages, the google glog library is a good leveled logging solution
    (https://github.com/golang/glog)
- **Data Persistence**: This implementation does not use a database or other
    persistent data store to save state. In a production implementation of this
    system, data persistence would be necessary to allow for code deployment,
    scaling, and server restarts. 

	
