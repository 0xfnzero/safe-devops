package main

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHealthUsesVerifiedTLSEndpoint(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			w.WriteHeader(503)
		}
	}))
	defer server.Close()
	cert, e := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if e != nil {
		t.Fatal(e)
	}
	ca := filepath.Join(t.TempDir(), "ca.crt")
	if e = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0600); e != nil {
		t.Fatal(e)
	}
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"safe-devops", "health", "--url", server.URL + "/readyz", "--ca", ca}
	if e = run(); e != nil {
		t.Fatal(e)
	}
	os.Args = []string{"safe-devops", "health", "--url", server.URL + "/bad", "--ca", ca}
	if run() == nil {
		t.Fatal("unready service accepted")
	}
	os.Args = []string{"safe-devops", "health", "--url", server.URL + "/readyz"}
	if run() == nil {
		t.Fatal("untrusted TLS certificate accepted")
	}
}
