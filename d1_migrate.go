//go:build !wasm

package goflare

import (
	"encoding/json"
	"fmt"
	"net/http"

	"webtyp.com/ddl"
)

// NewD1Migrator returns a ddl.Execer that runs schema migrations against a
// D1 database from CI or a developer machine — outside a Worker, where the
// webtyp/cloudflare/d1.NewEdge binding does not exist.
//
// It builds on CfClient rather than a second HTTP implementation: Bearer
// auth and {success,errors,result} envelope parsing are already correct in
// cloudflare.go's parseCFResponse. This repo is where that belongs — see
// webtyp/cloudflare/AGENTS.md, "no tooling code, ever, regardless of
// build tag".
//
// Usage:
//
//	conn, err := goflare.NewD1Migrator(accountID, databaseID, apiToken)
//	err = ddl.New(conn, sqlt.NewCompiler()).Sync(models...)
func NewD1Migrator(accountID, databaseID, apiToken string) (ddl.Execer, error) {
	if accountID == "" || databaseID == "" || apiToken == "" {
		return nil, fmt.Errorf("goflare: NewD1Migrator requires accountID, databaseID and apiToken")
	}
	client := &CfClient{Token: apiToken, BaseURL: cfAPIBase, HttpClient: http.DefaultClient}
	return NewD1MigratorFromClient(client, accountID, databaseID), nil
}

// NewD1MigratorFromClient is the test seam: it takes an already-constructed
// CfClient (whose BaseURL can point at an httptest.Server) instead of
// building one from real credentials.
func NewD1MigratorFromClient(client *CfClient, accountID, databaseID string) ddl.Execer {
	return &d1Migrator{
		client: client,
		path:   fmt.Sprintf("/accounts/%s/d1/database/%s/query", accountID, databaseID),
	}
}

// d1Migrator carries no Compiler: ddl.New takes the DDL compiler as its
// second argument (sqlt.NewCompiler()), and without storage.Compiler this
// type cannot be mistaken for a queryable connection.
type d1Migrator struct {
	client *CfClient
	path   string
}

// Exec POSTs one statement to D1's Query API via CfClient and reports
// whether it applied. CfClient.post already decodes the {success,errors,
// result} envelope and returns a descriptive error on failure — Exec adds
// nothing of its own beyond the request body.
func (m *d1Migrator) Exec(query string, args ...any) error {
	params := make([]any, len(args))
	copy(params, args)
	body, err := json.Marshal(struct {
		SQL    string `json:"sql"`
		Params []any  `json:"params"`
	}{SQL: query, Params: params})
	if err != nil {
		return err
	}
	_, err = m.client.post(m.path, body)
	return err
}

var _ ddl.Execer = (*d1Migrator)(nil)
