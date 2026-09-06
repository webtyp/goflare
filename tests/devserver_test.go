//go:build !wasm

package goflare_test

import (
	"io"
	"net/http"
	"testing"
	"time"

	"webtyp.com/goflare/devserver"
	"webtyp.com/router"
	"webtyp.com/server/httpd"
)

func TestDevServer(t *testing.T) {
	s := devserver.New(httpd.Config{
		Port: "9999",
	})

	r := s.Router()
	r.Get("/hello", func(ctx router.Context) {
		ctx.Write([]byte("world"))
	}).Public()

	go func() {
		if err := s.ListenAndServe(); err != nil {
			// Expect server closed error on shutdown
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:9999/hello")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "world" {
		t.Errorf("expected world, got %s", string(body))
	}
}
