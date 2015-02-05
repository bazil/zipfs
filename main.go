package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/net/context"
)

// We assume the zip file contains entries for directories too.

var progName = filepath.Base(os.Args[0])

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", progName)
	fmt.Fprintf(os.Stderr, "  %s ZIP MOUNTPOINT\n", progName)
	flag.PrintDefaults()
}

func main() {
	log.SetFlags(0)
	log.SetPrefix(progName + ": ")

	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 2 {
		usage()
		os.Exit(2)
	}
	path := flag.Arg(0)
	mountpoint := flag.Arg(1)
	if err := mount(path, mountpoint); err != nil {
		log.Fatal(err)
	}
}

func mount(path, mountpoint string) error {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer archive.Close()

	c, err := fuse.Mount(mountpoint)
	if err != nil {
		return err
	}
	defer c.Close()

	filesys := &FS{
		archive: &archive.Reader,
	}
	if err := fs.Serve(c, filesys); err != nil {
		return err
	}

	// check if the mount process has an error to report
	<-c.Ready
	if err := c.MountError; err != nil {
		return err
	}

	return nil
}

type FS struct {
	archive *zip.Reader
}

var _ fs.FS = (*FS)(nil)

func (f *FS) Root() (fs.Node, fuse.Error) {
	n := &Dir{
		archive: f.archive,
	}
	return n, nil
}

type Dir struct {
	archive *zip.Reader
	// nil for the root directory, which has no entry in the zip
	file *zip.File
}

var _ fs.Node = (*Dir)(nil)

func zipAttr(f *zip.File) fuse.Attr {
	return fuse.Attr{
		Size:   f.UncompressedSize64,
		Mode:   f.Mode(),
		Mtime:  f.ModTime(),
		Ctime:  f.ModTime(),
		Crtime: f.ModTime(),
	}
}

func (d *Dir) Attr() fuse.Attr {
	if d.file == nil {
		// root directory
		return fuse.Attr{Mode: os.ModeDir | 0755}
	}
	return zipAttr(d.file)
}

var _ = fs.NodeRequestLookuper(&Dir{})

func (d *Dir) Lookup(ctx context.Context, req *fuse.LookupRequest, resp *fuse.LookupResponse) (fs.Node, fuse.Error) {
	path := req.Name
	if d.file != nil {
		path = d.file.Name + path
	}
	for _, f := range d.archive.File {
		switch {
		case f.Name == path:
			child := &File{
				file: f,
			}
			return child, nil
		case f.Name[:len(f.Name)-1] == path && f.Name[len(f.Name)-1] == '/':
			child := &Dir{
				archive: d.archive,
				file:    f,
			}
			return child, nil
		}
	}
	return nil, fuse.ENOENT
}

var _ = fs.HandleReadDirer(&Dir{})

func (d *Dir) ReadDir(ctx context.Context) ([]fuse.Dirent, fuse.Error) {
	prefix := ""
	if d.file != nil {
		prefix = d.file.Name
	}

	var res []fuse.Dirent
	for _, f := range d.archive.File {
		if !strings.HasPrefix(f.Name, prefix) {
			continue
		}
		name := f.Name[len(prefix):]
		if name == "" {
			// the dir itself, not a child
			continue
		}
		if strings.ContainsRune(name[:len(name)-1], '/') {
			// contains slash in the middle -> is in a deeper subdir
			continue
		}
		var de fuse.Dirent
		if name[len(name)-1] == '/' {
			// directory
			name = name[:len(name)-1]
			de.Type = fuse.DT_Dir
		}
		de.Name = name
		res = append(res, de)
	}
	return res, nil
}

type File struct {
	file *zip.File
}

var _ fs.Node = (*File)(nil)

func (f *File) Attr() fuse.Attr {
	return zipAttr(f.file)
}

var _ = fs.NodeOpener(&File{})

func (f *File) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, fuse.Error) {
	r, err := f.file.Open()
	if err != nil {
		return nil, err
	}
	// individual entries inside a zip file are not seekable
	resp.Flags |= fuse.OpenNonSeekable
	return &FileHandle{r: r}, nil
}

type FileHandle struct {
	r io.ReadCloser
}

var _ fs.Handle = (*FileHandle)(nil)

var _ fs.HandleReleaser = (*FileHandle)(nil)

func (fh *FileHandle) Release(ctx context.Context, req *fuse.ReleaseRequest) fuse.Error {
	return fh.r.Close()
}

var _ = fs.HandleReader(&FileHandle{})

func (fh *FileHandle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) fuse.Error {
	// We don't actually enforce Offset to match where previous read
	// ended. Maybe we should, but that would mean'd we need to track
	// it. The kernel *should* do it for us, based on the
	// fuse.OpenNonSeekable flag.
	buf := make([]byte, req.Size)
	n, err := fh.r.Read(buf)
	resp.Data = buf[:n]
	return err
}
