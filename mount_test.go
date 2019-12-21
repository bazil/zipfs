package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"bazil.org/fuse/fs/fstestutil"
	"bazil.org/fuse/fs/fstestutil/spawntest"
	"bazil.org/fuse/fs/fstestutil/spawntest/httpjson"
	"golang.org/x/net/context"
)

var helpers spawntest.Registry

func TestMain(m *testing.M) {
	helpers.AddFlag(flag.CommandLine)
	flag.Parse()
	helpers.RunIfNeeded()
	os.Exit(m.Run())
}

func doSeek(ctx context.Context, path string) (*struct{}, error) {
	f, err := os.Open(filepath.Join(path, "greeting"))
	if err != nil {
		return nil, fmt.Errorf("cannot open greeting: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 1)
	if _, err := f.Read(buf); err != nil {
		return nil, fmt.Errorf("cannot read from greeting: %v", err)
	}

	if _, err := f.Seek(0, io.SeekStart); !errors.Is(err, syscall.ESPIPE) {
		return nil, fmt.Errorf("wrong error: %v", err)
	}
	return &struct{}{}, nil
}

var seekHelper = helpers.Register("seek", httpjson.ServePOST(doSeek))

func TestSeek(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var zipData bytes.Buffer
	{
		w := zip.NewWriter(&zipData)
		f, err := w.Create("greeting")
		if err != nil {
			t.Fatalf("cannot create zip: %v", err)
		}
		if _, err := f.Write([]byte("hello, world\n")); err != nil {
			t.Fatalf("cannot write to zip: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("cannot finalize zip: %v", err)
		}
	}

	// TODO refactor to share setup
	r, err := zip.NewReader(bytes.NewReader(zipData.Bytes()), int64(zipData.Len()))
	if err != nil {
		t.Fatalf("cannot read zip: %v", err)
	}

	filesys := &FS{
		archive: r,
	}
	mnt, err := fstestutil.MountedT(t, filesys, nil)
	if err != nil {
		t.Fatalf("cannot mount zipfs: %v", err)
	}
	defer mnt.Close()
	control := seekHelper.Spawn(ctx, t)
	defer control.Close()

	var nothing struct{}
	if err := control.JSON("/").Call(ctx, mnt.Dir, &nothing); err != nil {
		t.Fatalf("calling helper: %v", err)
	}
}
