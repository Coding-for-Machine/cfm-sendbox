package pkgman

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Coding-for-Machine/cfm-sendbox/internal/config"
)

func TestListMergesIndexes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("python,3.15.2,aaa,http://x/a.tgz\nlua,5.5.1,bbb,http://x/b.tgz\n"))
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("python,3.15.2,zzz,http://y/dup.tgz\npython,3.12.0,ccc,http://y/c.tgz\nbroken\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	m := New(&config.Config{RepoURLs: []string{srv.URL + "/missing", srv.URL + "/a", srv.URL + "/b"}}, nil)
	pkgs, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 3 {
		t.Fatalf("want 3 packages, got %d", len(pkgs))
	}
	for _, p := range pkgs {
		if p.Language == "python" && p.Version.Original() == "3.15.2" && p.Checksum != "aaa" {
			t.Fatalf("first index must win, got checksum %s", p.Checksum)
		}
	}

	m = New(&config.Config{RepoURLs: []string{srv.URL + "/missing"}}, nil)
	if _, err := m.List(); err == nil {
		t.Fatal("expected error when no index is reachable")
	}
}
