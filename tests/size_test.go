//go:build !wasm

package goflare_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/goflare"
)

func TestMeasureWasm(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.wasm")

	// Highly compressible content (repeated bytes)
	data := bytes.Repeat([]byte("A"), 10000)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	sizes, err := goflare.MeasureWasm(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sizes.Raw != int64(len(data)) {
		t.Errorf("expected Raw = %d, got %d", len(data), sizes.Raw)
	}
	if sizes.Gzip >= sizes.Raw {
		t.Errorf("expected Gzip (%d) < Raw (%d)", sizes.Gzip, sizes.Raw)
	}
}

func TestCheckWasmSizeAlwaysReports(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "small.wasm")

	// 10 KiB
	data := bytes.Repeat([]byte("B"), 10*1024)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	var logs []string
	logFunc := func(args ...any) {
		logs = append(logs, fmt.Sprint(args...))
	}

	if err := goflare.CheckWasmSize(path, logFunc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(logs) != 1 {
		t.Fatalf("expected exactly 1 log line, got %d: %v", len(logs), logs)
	}

	line := logs[0]
	if !strings.Contains(line, "raw") || !strings.Contains(line, "gzip") {
		t.Errorf("expected report to contain 'raw' and 'gzip', got: %s", line)
	}
}

func TestCheckWasmSizeWarns(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "warn.wasm")

	// 300 KiB
	data := bytes.Repeat([]byte("C"), 300*1024)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	var logs []string
	logFunc := func(args ...any) {
		logs = append(logs, fmt.Sprint(args...))
	}

	if err := goflare.CheckWasmSize(path, logFunc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var foundWarn bool
	for _, l := range logs {
		if strings.Contains(l, "warning:") {
			foundWarn = true
			break
		}
	}
	if !foundWarn {
		t.Errorf("expected warning log line for 300 KiB file, got logs: %v", logs)
	}
}

func TestCheckWasmSizeAborts(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "large.wasm")

	// 1 MiB (1024 KiB > 900 KiB)
	data := bytes.Repeat([]byte("D"), 1024*1024)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	var logs []string
	logFunc := func(args ...any) {
		logs = append(logs, fmt.Sprint(args...))
	}

	err := goflare.CheckWasmSize(path, logFunc)
	if err == nil {
		t.Fatal("expected error for 1 MiB file, got nil")
	}

	if !strings.Contains(err.Error(), "budget") {
		t.Errorf("expected error message to contain 'budget', got: %v", err)
	}
}

func TestWasmSizeThresholdsFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.wasm")

	// 10 KiB file
	data := bytes.Repeat([]byte("E"), 10*1024)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	// WASM_MAX_SIZE_KIB=1 makes 10 KiB fail
	t.Setenv(goflare.EnvKeyWasmMaxSizeKiB, "1")
	err := goflare.CheckWasmSize(path, nil)
	if err == nil {
		t.Error("expected error when WASM_MAX_SIZE_KIB=1 for 10 KiB file")
	}

	// WASM_MAX_SIZE_KIB=0 disables max threshold
	t.Setenv(goflare.EnvKeyWasmMaxSizeKiB, "0")
	err = goflare.CheckWasmSize(path, nil)
	if err != nil {
		t.Errorf("expected success when WASM_MAX_SIZE_KIB=0, got: %v", err)
	}
}
