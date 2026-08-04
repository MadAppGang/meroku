// Command mkzip packages a compiled Lambda bootstrap into a deployment zip.
//
// Terraform's local-exec provisioner runs it instead of the `zip` CLI, which is
// absent from plenty of build images. Go is already a hard requirement for
// building the function, so this adds no new dependency and it sets the
// executable bit portably — a bootstrap without 0755 fails at runtime with
// Runtime.InvalidEntrypoint.
//
//	go run ./tools/mkzip -in .build/dev/bootstrap -out .build/dev/ci_lambda.zip
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	in := flag.String("in", "", "path to the compiled bootstrap binary")
	out := flag.String("out", "", "path of the zip to write")
	flag.Parse()

	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "mkzip: both -in and -out are required")
		os.Exit(2)
	}

	if err := run(*in, *out); err != nil {
		fmt.Fprintf(os.Stderr, "mkzip: %v\n", err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	src, err := os.Open(in)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	info, err := src.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("%s is empty", in)
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}

	// Write to a temporary file and rename, so a concurrent reader never sees
	// a half-written archive.
	tmp, err := os.CreateTemp(filepath.Dir(out), ".mkzip-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	zw := zip.NewWriter(tmp)
	hdr := &zip.FileHeader{
		Name:     filepath.Base(in),
		Method:   zip.Deflate,
		Modified: info.ModTime(),
	}
	hdr.SetMode(0o755)

	w, err := zw.CreateHeader(hdr)
	if err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.Copy(w, src); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp makes the file 0600; the archive is not a secret and may be
	// read by a different user in CI.
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}

	return os.Rename(tmpName, out)
}
