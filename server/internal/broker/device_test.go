package broker

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"safeops/internal/core"
	"safeops/internal/device"
	"safeops/internal/store"
	"strconv"
	"sync"
	"testing"
	"time"
)

func deviceFixture(t *testing.T, cidrs, proxies []string) (*fixture, ed25519.PrivateKey) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	f := setup(t, func(c *core.Config, _ *core.AgentConfig, _ map[string]ed25519.PrivateKey) {
		c.DevicePolicy = &core.DevicePolicy{Required: true, Devices: []core.Device{{ID: core.DeviceID(key.Public().(ed25519.PublicKey)), Name: "workstation", PublicKey: device.Public(key), Principals: []string{"alice"}, AllowedCIDRs: cidrs}}, TrustedProxyCIDRs: proxies}
	})
	return f, key
}

func TestDeviceNonceSurvivesConcurrentInstancesAndRestart(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			o := store.Options{Driver: driver, Workspace: "device-" + core.ID()}
			if driver == "sqlite" {
				o.Path = filepath.Join(t.TempDir(), "device.db")
			} else {
				o.URL = os.Getenv("SAFEOPS_TEST_DATABASE_URL")
				if o.URL == "" {
					t.Skip("SAFEOPS_TEST_DATABASE_URL not configured")
				}
			}
			f, key := deviceFixture(t, nil, nil)
			open := func() *Broker {
				s, err := store.OpenDatabase(context.Background(), o)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { s.Close() })
				b, err := New(context.Background(), f.broker.Config, s, f.broker.Key, true)
				if err != nil {
					t.Fatal(err)
				}
				return b
			}
			first, second := open(), open()
			r := signedDeviceRequest(t, f, key, "GET", "/api/me", nil)
			r.RemoteAddr = "127.0.0.1:12345"
			codes := make(chan int, 12)
			var wg sync.WaitGroup
			for n := 0; n < 12; n++ {
				wg.Add(1)
				go func(n int) {
					defer wg.Done()
					w := httptest.NewRecorder()
					b := first
					if n%2 == 1 {
						b = second
					}
					b.ServeHTTP(w, r.Clone(r.Context()))
					codes <- w.Code
				}(n)
			}
			wg.Wait()
			close(codes)
			accepted := 0
			for code := range codes {
				if code == 200 {
					accepted++
				} else if code != 409 {
					t.Fatalf("unexpected concurrent response: %d", code)
				}
			}
			if accepted != 1 {
				t.Fatalf("same signed request accepted %d times", accepted)
			}
			first.Store.Close()
			second.Store.Close()
			restarted := open()
			w := httptest.NewRecorder()
			restarted.ServeHTTP(w, r.Clone(r.Context()))
			if w.Code != 409 {
				t.Fatalf("restart accepted consumed proof: %d", w.Code)
			}
			fresh := signedDeviceRequest(t, f, key, "GET", "/api/me", nil)
			fresh.RemoteAddr = r.RemoteAddr
			w = httptest.NewRecorder()
			restarted.ServeHTTP(w, fresh)
			if w.Code != 200 {
				t.Fatalf("restart rejected fresh proof: %d", w.Code)
			}
		})
	}
}

func TestDevicePreloginDoesNotAuthorizeOtherMembersOrTeams(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	var brokers []*Broker
	for _, workspace := range []string{"alpha", "beta"} {
		f := setup(t, func(c *core.Config, _ *core.AgentConfig, _ map[string]ed25519.PrivateKey) {
			c.WorkspaceID = workspace
			c.Projects = []core.Project{{ID: "project", Name: "Project", Members: []string{"alice", "bob", "carol"}}}
			for n := range c.Targets {
				c.Targets[n].ProjectID = "project"
			}
			for n := range c.Principals {
				c.Principals[n].TokenHash = core.Hash([]byte(workspace + "-" + c.Principals[n].ID))
			}
			for n := range c.Agents {
				c.Agents[n].ID = workspace + "-" + c.Agents[n].ID
				c.Agents[n].TokenHash = core.Hash([]byte(c.Agents[n].ID))
			}
			c.DevicePolicy = &core.DevicePolicy{Required: true}
			if workspace == "alpha" {
				c.DevicePolicy.Devices = []core.Device{{ID: core.DeviceID(key.Public().(ed25519.PublicKey)), PublicKey: device.Public(key), Principals: []string{"alice"}}}
			}
		})
		brokers = append(brokers, f.broker)
	}
	router, err := NewRouter(brokers, true)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, credential string, signed bool, want int) {
		t.Helper()
		r := httptest.NewRequest(method, path, nil)
		if credential != "" {
			r.Header.Set("Authorization", "Bearer "+credential)
		}
		if signed {
			device.SignHeaders(r, key, nil, strconv.FormatInt(time.Now().Unix(), 10), core.ID())
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", credential, path, w.Code, want, w.Body.String())
		}
	}
	call("POST", "/api/device/verify", "", true, 200)
	call("GET", "/api/me", "alpha-alice", false, 403)
	call("GET", "/api/me", "alpha-alice", true, 200)
	call("GET", "/api/me", "alpha-bob", true, 403)
	call("GET", "/api/me", "beta-alice", true, 403)
	call("GET", "/api/me", "invalid-token", true, 401)
}

func TestDevicePreloginRateLimitIgnoresSourcePort(t *testing.T) {
	f, _ := deviceFixture(t, nil, nil)
	for n := 0; n < 241; n++ {
		r := httptest.NewRequest("POST", "/api/device/verify", nil)
		r.RemoteAddr = fmt.Sprintf("192.0.2.1:%d", 10000+n)
		if got := f.broker.allowDevicePrelogin(r); got != (n < 240) {
			t.Fatalf("source port bypass at request %d", n)
		}
	}
}
func signedDeviceRequest(t *testing.T, f *fixture, key ed25519.PrivateKey, method, path string, body []byte) *http.Request {
	t.Helper()
	r, e := http.NewRequest(method, f.server.URL+path, bytes.NewReader(body))
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("Authorization", "Bearer alice")
	r.Header.Set("Content-Type", "application/json")
	device.SignHeaders(r, key, body, strconv.FormatInt(time.Now().Unix(), 10), core.ID())
	return r
}
func assertDeviceStatus(t *testing.T, r *http.Request, want int) {
	t.Helper()
	res, e := http.DefaultClient.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode != want {
		t.Fatalf("device request %s %s: got %d, want %d", r.Method, r.URL.Path, res.StatusCode, want)
	}
}
func TestEveryRequestRequiresFreshIdentityBoundDeviceProof(t *testing.T) {
	f, key := deviceFixture(t, nil, nil)
	f.call("alice", "GET", "/api/me", nil, 403, nil)
	valid := signedDeviceRequest(t, f, key, "GET", "/api/me", nil)
	replay := valid.Clone(valid.Context())
	assertDeviceStatus(t, valid, 200)
	assertDeviceStatus(t, replay, 409)
	for _, attack := range []string{"path", "method", "credential", "body", "future", "expired", "nonce", "unknown-machine"} {
		t.Run(attack, func(t *testing.T) {
			body := []byte(`{"target":"t1","action":"status","reason":"investigate safely"}`)
			r := signedDeviceRequest(t, f, key, "POST", "/api/requests", body)
			switch attack {
			case "path":
				r.URL.Path = "/api/requests/other/revoke"
			case "method":
				r.Method = "GET"
			case "credential":
				r.Header.Set("Authorization", "Bearer bob")
			case "body":
				r.Body = http.NoBody
				r.ContentLength = 0
			case "future":
				device.SignHeaders(r, key, body, strconv.FormatInt(time.Now().Unix()+60, 10), core.ID())
			case "expired":
				device.SignHeaders(r, key, body, strconv.FormatInt(time.Now().Unix()-60, 10), core.ID())
			case "nonce":
				r.Header.Set("X-Safe-Devops-Nonce", "invalid")
			case "unknown-machine":
				_, other, _ := ed25519.GenerateKey(rand.Reader)
				device.SignHeaders(r, other, body, strconv.FormatInt(time.Now().Unix(), 10), core.ID())
			}
			assertDeviceStatus(t, r, 403)
		})
	}
	r := signedDeviceRequest(t, f, key, "POST", "/api/requests", []byte(`{"target":"t1","action":"status","reason":"investigate safely"}`))
	assertDeviceStatus(t, r, 200)
}
func TestOptionalIPAndTrustedProxyChecks(t *testing.T) {
	for _, tc := range []struct {
		name           string
		cidrs, proxies []string
		xff            string
		want           int
	}{
		{name: "optional", want: 200},
		{name: "matching", cidrs: []string{"127.0.0.0/8"}, want: 200},
		{name: "denied", cidrs: []string{"10.0.0.0/8"}, want: 403},
		{name: "spoofed-forwarding", cidrs: []string{"10.0.0.0/8"}, xff: "10.0.1.2", want: 403},
		{name: "trusted", cidrs: []string{"10.0.0.0/8"}, proxies: []string{"127.0.0.1/32"}, xff: "10.0.1.2", want: 200},
		{name: "missing-trusted-chain", cidrs: []string{"10.0.0.0/8"}, proxies: []string{"127.0.0.1/32"}, want: 403},
		{name: "rightmost-client", cidrs: []string{"10.0.0.0/8"}, proxies: []string{"127.0.0.1/32"}, xff: "10.0.1.2, 198.51.100.9", want: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, key := deviceFixture(t, tc.cidrs, tc.proxies)
			r := signedDeviceRequest(t, f, key, "GET", "/api/me", nil)
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			assertDeviceStatus(t, r, tc.want)
		})
	}
}
