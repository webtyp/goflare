//go:build !wasm

package goflare_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"webtyp.com/ddl"
	"webtyp.com/goflare"
	"webtyp.com/model"
	"webtyp.com/sqlt"
)

type d1MigrateMockModel struct {
	id   string
	name string
}

func (m *d1MigrateMockModel) Schema() model.Fields {
	return model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.Text()},
	}
}

func (m *d1MigrateMockModel) Pointers() []any {
	return []any{&m.id, &m.name}
}

func (m *d1MigrateMockModel) ModelName() string {
	return "mock_models"
}

func (m *d1MigrateMockModel) TableName() string {
	return "mock_models"
}

func (m *d1MigrateMockModel) EncodeFields(w model.FieldWriter) {}

func (m *d1MigrateMockModel) DecodeFields(r model.FieldReader) {}

func (m *d1MigrateMockModel) IsNil() bool {
	return m == nil
}

func TestD1Migrator_Validation(t *testing.T) {
	if _, err := goflare.NewD1Migrator("", "db", "tok"); err == nil {
		t.Fatal("expected error for empty accountID")
	}
	if _, err := goflare.NewD1Migrator("acc", "", "tok"); err == nil {
		t.Fatal("expected error for empty databaseID")
	}
	if _, err := goflare.NewD1Migrator("acc", "db", ""); err == nil {
		t.Fatal("expected error for empty apiToken")
	}
}

func TestD1Migrator_ExecSuccessAndAuthHeader(t *testing.T) {
	var receivedAuth string
	var receivedBody struct {
		SQL    string `json:"sql"`
		Params []any  `json:"params"`
	}

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed reading request body: %v", err)
		}
		if err := json.Unmarshal(bodyBytes, &receivedBody); err != nil {
			t.Fatalf("failed unmarshaling request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true, "errors": [], "result": []}`))
	})
	defer server.Close()

	client := &goflare.CfClient{Token: "my-secret-token", BaseURL: server.URL, HttpClient: http.DefaultClient}
	conn := goflare.NewD1MigratorFromClient(client, "acc", "db")

	if err := conn.Exec("CREATE TABLE sample (id TEXT PRIMARY KEY);"); err != nil {
		t.Fatalf("unexpected Exec error: %v", err)
	}
	if receivedAuth != "Bearer my-secret-token" {
		t.Errorf("expected Authorization header 'Bearer my-secret-token', got %q", receivedAuth)
	}
	if receivedBody.SQL != "CREATE TABLE sample (id TEXT PRIMARY KEY);" {
		t.Errorf("expected sql 'CREATE TABLE sample (id TEXT PRIMARY KEY);', got %q", receivedBody.SQL)
	}
}

func TestD1Migrator_ErrorEnvelope(t *testing.T) {
	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": false, "errors": [{"code": 7500, "message": "syntax error near 'SELCT'"}], "result": []}`))
	})
	defer server.Close()

	client := &goflare.CfClient{Token: "token", BaseURL: server.URL, HttpClient: http.DefaultClient}
	conn := goflare.NewD1MigratorFromClient(client, "acc", "db")

	err := conn.Exec("SELCT 1;")
	if err == nil {
		t.Fatal("expected Exec error, got nil")
	}
	if !strings.Contains(err.Error(), "syntax error near 'SELCT'") {
		t.Errorf("expected error message to contain 'syntax error near 'SELCT'', got %q", err.Error())
	}
}

func TestD1Migrator_EndToEndDDLSync(t *testing.T) {
	var executedQueries []string

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed reading request body: %v", err)
		}
		var payload struct {
			SQL string `json:"sql"`
		}
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			t.Fatalf("failed unmarshaling request body: %v", err)
		}
		executedQueries = append(executedQueries, payload.SQL)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true, "errors": [], "result": []}`))
	})
	defer server.Close()

	client := &goflare.CfClient{Token: "token", BaseURL: server.URL, HttpClient: http.DefaultClient}
	conn := goflare.NewD1MigratorFromClient(client, "acc", "db")
	db := ddl.New(conn, sqlt.NewCompiler())

	if err := db.Sync(&d1MigrateMockModel{}); err != nil {
		t.Fatalf("unexpected Sync error: %v", err)
	}
	if len(executedQueries) == 0 {
		t.Fatal("expected executed queries, got 0")
	}

	hasCreateTable := false
	for _, q := range executedQueries {
		if strings.Contains(q, "CREATE TABLE") {
			hasCreateTable = true
			break
		}
	}
	if !hasCreateTable {
		t.Errorf("expected a CREATE TABLE query, got queries: %v", executedQueries)
	}
}
