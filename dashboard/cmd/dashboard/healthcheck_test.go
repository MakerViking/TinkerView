package main

import "testing"

func TestProbeAddr(t *testing.T) {
	cases := map[string]string{
		":8088":          "127.0.0.1:8088",
		"0.0.0.0:9000":   "127.0.0.1:9000",
		"[::]:8088":      "127.0.0.1:8088",
		"10.0.0.5:8088":  "10.0.0.5:8088",
		"127.0.0.1:8088": "127.0.0.1:8088",
		"garbage":        "127.0.0.1:8088",
	}
	for in, want := range cases {
		if got := probeAddr(in); got != want {
			t.Errorf("probeAddr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListenPort(t *testing.T) {
	if got := listenPort(":8088"); got != "8088" {
		t.Errorf("listenPort(:8088) = %q", got)
	}
	if got := listenPort("1.2.3.4:99"); got != "99" {
		t.Errorf("listenPort(1.2.3.4:99) = %q", got)
	}
}
