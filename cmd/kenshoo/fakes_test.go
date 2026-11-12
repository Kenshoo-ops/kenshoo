package main

import (
	"context"

	"github.com/Kenshoo-ops/kenshoo/internal/restorer"
)

// fakeDB stands in for a provisioned database.
type fakeDB struct {
	major  int
	scalar string
	drops  *int
}

func (f fakeDB) Name() string     { return "kenshoo_fake" }
func (f fakeDB) DSN() string      { return "postgres://u:hunter2@db.example/kenshoo_fake" }
func (f fakeDB) ServerMajor() int { return f.major }
func (f fakeDB) Drop(context.Context) error {
	if f.drops != nil {
		*f.drops++
	}
	return nil
}
func (f fakeDB) QueryScalar(context.Context, string) (string, error) { return f.scalar, nil }

const fakeAdminDSN = "postgres://admin:hunter2@127.0.0.1:1/postgres"

// testDeps is the production wiring with everything that touches the network replaced.
func testDeps(newStore storeFactory) deps {
	return deps{
		newStore:    newStore,
		newRestorer: restorer.New,
		provision: func(context.Context, string) (database, error) {
			return fakeDB{major: 16, scalar: "3"}, nil
		},
		getenv: func(key string) string {
			if key == adminDSNEnv {
				return fakeAdminDSN
			}
			return ""
		},
	}
}
