//go:build windows

package main

import (
	"archive/zip"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTimeText(t *testing.T) {
	valid := map[string]float64{
		"00:00:00.000":  0,
		"00:01:02.345":  62.345,
		"01:02:03.456":  3723.456,
		"100:59:59.999": 363599.999,
	}
	for input, want := range valid {
		got, err := parseTimeText(input)
		if err != nil || math.Abs(got-want) > 1e-9 {
			t.Fatalf("parseTimeText(%q) = %v, %v; want %v", input, got, err, want)
		}
	}

	invalid := []string{
		"abc:00:00.000",
		"00:x:00.000",
		"00:00:xx",
		"00:60:00.000",
		"00:00:60.000",
		"00:00:nan",
		"00:00:+Inf",
		"00:00:-1",
		"00:00",
	}
	for _, input := range invalid {
		if _, err := parseTimeText(input); err == nil {
			t.Fatalf("parseTimeText(%q) unexpectedly accepted invalid input", input)
		}
	}
}

func TestFormatTimeRoundTrip(t *testing.T) {
	values := []float64{0, 0.001, 1.999, 62.345, 3723.456, 86399.999}
	for _, want := range values {
		text := formatTime(want)
		got, err := parseTimeText(text)
		if err != nil || math.Abs(got-want) > 0.001 {
			t.Fatalf("round trip %v -> %q -> %v (%v)", want, text, got, err)
		}
	}
}

func TestToolsComplete(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"ffmpeg.exe", "ffprobe.exe", "ffplay.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !toolsComplete(dir) {
		t.Fatal("toolsComplete returned false for all required files")
	}
	if err := os.Remove(filepath.Join(dir, "ffplay.exe")); err != nil {
		t.Fatal(err)
	}
	if toolsComplete(dir) {
		t.Fatal("toolsComplete returned true with ffplay.exe missing")
	}
}

func TestExtractRuntimeBin(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "runtime.zip")
	outDir := filepath.Join(t.TempDir(), "bin")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	entries := map[string][]byte{
		"ffmpeg-n9.0-test/bin/ffmpeg.exe":  []byte("ffmpeg"),
		"ffmpeg-n9.0-test/bin/ffprobe.exe": []byte("ffprobe"),
		"ffmpeg-n9.0-test/bin/ffplay.exe":  []byte("ffplay"),
		"ffmpeg-n9.0-test/bin/avcodec.dll": []byte("dll"),
		"ffmpeg-n9.0-test/README.txt":      []byte("ignore"),
	}
	for name, data := range entries {
		w, err := zw.Create(name)
		if err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if err := extractRuntimeBin(zipPath, outDir); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ffmpeg.exe", "ffprobe.exe", "ffplay.exe", "avcodec.dll"} {
		if _, err := os.Stat(filepath.Join(outDir, want)); err != nil {
			t.Fatalf("missing extracted file %s: %v", want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "README.txt")); err == nil {
		t.Fatal("non-bin file was extracted")
	}
}

func TestReplaceOutputFile(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "result.mp4")
	tmp := output + ".part.mp4"
	if err := os.WriteFile(output, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := replaceOutputFile(tmp, output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("output content = %q, want %q", data, "new")
	}
	if _, err := os.Stat(tmp); err == nil {
		t.Fatal("temporary output still exists")
	}
	if _, err := os.Stat(output + ".old"); err == nil {
		t.Fatal("backup file still exists")
	}
}

func TestBuildExportArgs(t *testing.T) {
	copyArgs := buildExportArgs("in.mp4", "out.part.mp4", 1.25, 2.5, false, true)
	exactArgs := buildExportArgs("in.mp4", "out.part.mp4", 1.25, 2.5, true, true)
	joinedCopy := strings.Join(copyArgs, " ")
	joinedExact := strings.Join(exactArgs, " ")
	for _, want := range []string{"-c:v copy", "-c:a copy", "-avoid_negative_ts make_zero"} {
		if !strings.Contains(joinedCopy, want) {
			t.Fatalf("copy args missing %q: %s", want, joinedCopy)
		}
	}
	for _, want := range []string{"libx264", "-crf 18", "-movflags +faststart"} {
		if !strings.Contains(joinedExact, want) {
			t.Fatalf("exact args missing %q: %s", want, joinedExact)
		}
	}
	if copyArgs[len(copyArgs)-1] != "out.part.mp4" || exactArgs[len(exactArgs)-1] != "out.part.mp4" {
		t.Fatal("output path is not the final ffmpeg argument")
	}
}
