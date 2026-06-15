package openbd

import (
	"strings"
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions.
// No network is touched.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "openbd" {
		t.Errorf("Scheme = %q, want openbd", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "openbd" {
		t.Errorf("Identity.Binary = %q, want openbd", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	typ, id, err := Domain{}.Classify("9784873115658")
	if err != nil {
		t.Fatalf("Classify error: %v", err)
	}
	if typ != "book" {
		t.Errorf("type = %q, want book", typ)
	}
	if id != "9784873115658" {
		t.Errorf("id = %q, want 9784873115658", id)
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("book", "9784873115658")
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}
	if !strings.Contains(got, "9784873115658") {
		t.Errorf("Locate = %q, want it to contain the ISBN", got)
	}
	if !strings.HasPrefix(got, "https://openbd.jp/p/") {
		t.Errorf("Locate = %q, want https://openbd.jp/p/...", got)
	}
}
