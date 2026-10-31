package broker

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMTLSIdentityCannotBeSpoofedWithBearerOrHeader(t *testing.T) {
	f := setup(t)
	f.server.Close()
	f.broker.Demo = false
	caPub, caKey, _ := ed25519.GenerateKey(rand.Reader)
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test agent CA"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caBytes, e := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPub, caKey)
	if e != nil {
		t.Fatal(e)
	}
	ca, e := x509.ParseCertificate(caBytes)
	if e != nil {
		t.Fatal(e)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	certificate := func(cn string) tls.Certificate {
		pub, key, _ := ed25519.GenerateKey(rand.Reader)
		tmpl := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: cn}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		der, e := x509.CreateCertificate(rand.Reader, tmpl, ca, pub, caKey)
		if e != nil {
			t.Fatal(e)
		}
		keyBytes, _ := x509.MarshalPKCS8PrivateKey(key)
		cert, e := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}))
		if e != nil {
			t.Fatal(e)
		}
		return cert
	}
	srv := httptest.NewUnstartedServer(f.broker)
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.VerifyClientCertIfGiven, ClientCAs: pool}
	srv.StartTLS()
	defer srv.Close()
	for _, tc := range []struct {
		name, cn string
		status   int
	}{{"no certificate", "", 401}, {"valid agent certificate", "a1", 200}, {"unknown certificate identity", "not-an-agent", 401}} {
		t.Run(tc.name, func(t *testing.T) {
			transport := srv.Client().Transport.(*http.Transport).Clone()
			transport.TLSClientConfig = transport.TLSClientConfig.Clone()
			if tc.cn != "" {
				transport.TLSClientConfig.Certificates = []tls.Certificate{certificate(tc.cn)}
			}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport}
			req, _ := http.NewRequest("POST", srv.URL+"/api/agent/poll", nil)
			req.Header.Set("Authorization", "Bearer agent")
			req.Header.Set("X-SSL-Client-CN", "a1")
			req.Header.Set("X-Forwarded-Client-Cert", "CN=a1")
			res, e := client.Do(req)
			if e != nil {
				t.Fatal(e)
			}
			defer res.Body.Close()
			if res.StatusCode != tc.status {
				t.Fatalf("got %d want %d", res.StatusCode, tc.status)
			}
		})
	}
}
