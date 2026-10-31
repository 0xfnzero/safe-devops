package store

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"safeops/internal/core"
)

func TestPostgresTransactionsAcrossInstances(t *testing.T) {
	url := os.Getenv("SAFEOPS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set SAFEOPS_TEST_DATABASE_URL to a dedicated *_test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, e := sql.Open("pgx", url)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var name string
	if e = db.QueryRowContext(ctx, "SELECT current_database()").Scan(&name); e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("refusing to reset a database without _test suffix")
	}
	if _, e = db.ExecContext(ctx, "DROP TABLE IF EXISTS safeops_state"); e != nil {
		t.Fatal(e)
	}
	a, e := Open(ctx, "", url)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := Open(ctx, "", url)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for n := 0; n < 20; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s := a
			if n%2 == 1 {
				s = b
			}
			errs <- s.Update(ctx, func(st *core.State) error {
				st.Heartbeats["counter"]++
				Append(st, time.Now(), "integration", "increment", "", "")
				return nil
			})
		}(n)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if e = a.Update(ctx, func(st *core.State) error {
		if st.Heartbeats["counter"] != 20 || len(st.Audit) != 20 {
			t.Fatalf("lost update: %+v", st)
		}
		return VerifyAudit(st.Audit)
	}); e != nil {
		t.Fatal(e)
	}
}
