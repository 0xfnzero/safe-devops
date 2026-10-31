package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"safeops/internal/core"
	"strconv"
	"strings"
	"time"
)

type authority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func certificate(name string, hosts []string, client bool, parent *authority) (*authority, []byte, error) {
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return nil, nil, e
	}
	serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if e != nil {
		return nil, nil, e
	}
	c := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: name}, NotBefore: time.Now().Add(-5 * time.Minute), NotAfter: time.Now().AddDate(1, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			c.IPAddresses = append(c.IPAddresses, ip)
		} else {
			c.DNSNames = append(c.DNSNames, host)
		}
	}
	issuer, signer := c, key
	if parent == nil {
		c.IsCA = true
		c.MaxPathLenZero = true
		c.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
		c.NotAfter = time.Now().AddDate(5, 0, 0)
	} else {
		issuer, signer = parent.cert, parent.key
		if client {
			c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
		} else {
			c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		}
	}
	der, e := x509.CreateCertificate(rand.Reader, c, issuer, &key.PublicKey, signer)
	if e != nil {
		return nil, nil, e
	}
	parsed, e := x509.ParseCertificate(der)
	if e != nil {
		return nil, nil, e
	}
	private, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		return nil, nil, e
	}
	return &authority{parsed, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), nil
}

// Each service receives only its own files. CA private keys and human tokens are
// outside every container mount; the human signing key is never generated here.
func initCompose(dir, workspace, project, brokerURL, webURL, approval, database string, diagnosticRoot ...string) error {
	web, e := url.Parse(webURL)
	if e != nil || web.Scheme != "https" || web.Hostname() == "" || web.User != nil || web.RawQuery != "" || web.Fragment != "" || (web.Path != "" && web.Path != "/") {
		return errors.New("--web-url must be an HTTPS origin")
	}
	// Compose interpolation must not reinterpret paths or untrusted origin input.
	dir, e = filepath.Abs(dir)
	if e != nil {
		return e
	}
	if strings.ContainsAny(dir+brokerURL+webURL, "\r\n$'\"`\\") {
		return errors.New("paths and URLs cannot contain Compose interpolation or quoting characters")
	}
	brokerOrigin, e := url.Parse(brokerURL)
	if e != nil {
		return e
	}
	port := func(u *url.URL, def string) string {
		if u.Port() != "" {
			return u.Port()
		}
		return def
	}
	brokerPort, webPort := port(brokerOrigin, "443"), port(web, "443")
	if brokerPort == webPort {
		return errors.New("Broker and Web must use distinct host ports (for example 8443 and 443)")
	}
	for _, p := range []string{brokerPort, webPort} {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 {
			return errors.New("invalid published port")
		}
	}
	if e = initConfig(dir, workspace, project, brokerURL, approval, database, diagnosticRoot...); e != nil {
		return e
	}
	uid, gid := os.Getuid(), os.Getgid()
	if uid == 0 {
		uid, gid = 10001, 10001
	}
	for _, name := range []string{"broker", "web", "gateway", "connector", "credentials", "offline-ca", "postgres", "data/broker", "data/connector", "backups"} {
		if e = os.MkdirAll(filepath.Join(dir, name), 0700); e != nil {
			return e
		}
	}
	move := func(from, to string) error { return os.Rename(filepath.Join(dir, from), filepath.Join(dir, to)) }
	for _, pair := range [][2]string{{"broker.json", "broker/broker.json"}, {workspace + "-broker.key", "broker/" + workspace + "-broker.key"}, {"broker.env", "broker/broker.env"}, {"web.env", "web/web.env"}, {"connector.json", "connector/connector.json"}, {"developer.token", "credentials/developer.token"}, {"reviewer.token", "credentials/reviewer.token"}, {"auditor.token", "credentials/auditor.token"}} {
		if e = move(pair[0], pair[1]); e != nil {
			return e
		}
	}
	write := func(path string, data []byte) error { return os.WriteFile(filepath.Join(dir, path), data, 0600) }
	ca, key, e := certificate("safe-devops server CA", nil, false, nil)
	if e != nil {
		return e
	}
	if e = write("offline-ca/server-ca.key", key); e != nil {
		return e
	}
	clientCA, key, e := certificate("safe-devops Connector CA", nil, false, nil)
	if e != nil {
		return e
	}
	if e = write("offline-ca/connector-ca.key", key); e != nil {
		return e
	}
	for _, path := range []string{"offline-ca/server-ca.crt", "web/broker-ca.crt", "connector/broker-ca.crt", "credentials/ca.crt", "broker/db-ca.crt"} {
		if e = write(path, ca.pem); e != nil {
			return e
		}
	}
	for _, path := range []string{"offline-ca/connector-ca.crt", "broker/connector-ca.crt"} {
		if e = write(path, clientCA.pem); e != nil {
			return e
		}
	}
	for _, item := range []struct {
		dir, name, cn string
		hosts         []string
		client        bool
		ca            *authority
	}{
		{"broker", "broker", brokerOrigin.Hostname(), []string{"broker", brokerOrigin.Hostname()}, false, ca},
		{"gateway", "web", web.Hostname(), []string{web.Hostname()}, false, ca},
		{"connector", "connector", workspace + "-connector-01", nil, true, clientCA},
		{"postgres", "server", "postgres", []string{"postgres"}, false, ca},
	} {
		crt, private, e := certificate(item.cn, item.hosts, item.client, item.ca)
		if e != nil {
			return e
		}
		if e = write(item.dir+"/"+item.name+".crt", crt.pem); e != nil {
			return e
		}
		if e = write(item.dir+"/"+item.name+"-tls.key", private); e != nil {
			return e
		}
	}
	if e = write("web/web.env", []byte("SAFEOPS_API_ORIGIN=https://broker:8443\nNODE_EXTRA_CA_CERTS=/etc/safe-devops/broker-ca.crt\n")); e != nil {
		return e
	}
	if database == "postgres" {
		password := core.ID() + core.ID()
		if e = write("postgres/password", []byte(password)); e != nil {
			return e
		}
		dsn := url.URL{Scheme: "postgres", Host: "postgres:5432", Path: "/safeops", User: url.UserPassword("safeops", password)}
		q := url.Values{"sslmode": {"verify-full"}, "sslrootcert": {"/etc/safe-devops/db-ca.crt"}}
		dsn.RawQuery = q.Encode()
		if e = write("broker/broker.env", []byte("SAFEOPS_DATABASE_URL="+dsn.String()+"\n")); e != nil {
			return e
		}
	}
	if e = write("gateway/Caddyfile", []byte("{\n admin off\n auto_https off\n}\nhttp://127.0.0.1:8080 {\n respond /healthz \"ok\" 200\n}\n:8443 {\n tls /etc/safe-devops/web.crt /etc/safe-devops/web-tls.key\n reverse_proxy web:3000\n}\n")); e != nil {
		return e
	}
	env := fmt.Sprintf("SAFEOPS_CONFIG_DIR='%s'\nSAFEOPS_UID=%d\nSAFEOPS_GID=%d\nSAFEOPS_BROKER_PORT=%s\nSAFEOPS_WEB_PORT=%s\nCOMPOSE_PROJECT_NAME=safe-devops-%s\n", dir, uid, gid, brokerPort, webPort, workspace)
	if e = write(".env", []byte(env)); e != nil {
		return e
	}
	if e = write(".database", []byte(database+"\n")); e != nil {
		return e
	}
	if os.Getuid() == 0 {
		for _, sub := range []string{"broker", "web", "gateway", "connector", "data", "backups"} {
			if e = filepath.Walk(filepath.Join(dir, sub), func(path string, _ os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				return os.Chown(path, uid, gid)
			}); e != nil {
				return e
			}
		}
	}
	fmt.Printf("Compose configuration ready: %s\nTrust credentials/ca.crt on browser and approval devices. Move offline-ca to offline custody.\nStart: ./scripts/compose.sh %s up -d --build\nConnector bundle: %s/connector (install only on the target server).\n", dir, dir, dir)
	return nil
}
