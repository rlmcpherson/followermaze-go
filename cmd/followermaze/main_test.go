package main

import "testing"

func TestEnv(t *testing.T) {

	// verify environment variables are set
	for _, o := range opts {
		if o.val == "" {
			t.Fatalf("%s not initialized", o.env)
		}
	}
}
