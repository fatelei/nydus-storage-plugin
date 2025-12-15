package fs

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"syscall"

	"github.com/containerd/containerd/remotes/docker"
	fusefs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// blobFile streams a remote blob sequentially via containerd's resolver.
type blobFile struct {
	mu   sync.Mutex
	rc   io.ReadCloser
	size int64
	pos  int64
}

func newBlobFile(ctx context.Context, ref string, desc ocispec.Descriptor, hostsFn func(string) ([]docker.RegistryHost, error)) (*blobFile, error) {
	resolver := docker.NewResolver(docker.ResolverOptions{
		Hosts: func(host string) ([]docker.RegistryHost, error) {
			return hostsFn(host)
		},
	})
	fetcher, err := resolver.Fetcher(ctx, ref)
	if err != nil {
		return nil, err
	}
	r, err := fetcher.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}
	return &blobFile{rc: r, size: desc.Size, pos: 0}, nil
}

var _ = (fusefs.FileReader)((*blobFile)(nil))
var _ = (fusefs.FileReleaser)((*blobFile)(nil))
var _ = (fusefs.FileGetattrer)((*blobFile)(nil))

func (f *blobFile) Read(_ context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if off != f.pos {
		// Only sequential reads are supported in this initial implementation
		slog.Warn("non-sequential read on blob; returning EIO", "off", off, "pos", f.pos)
		return nil, syscall.EIO
	}
	n, err := io.ReadAtLeast(f.rc, dest, 1)
	if err == io.EOF {
		return fuse.ReadResultData(dest[:0]), 0
	}
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, syscall.EIO
	}
	f.pos += int64(n)
	return fuse.ReadResultData(dest[:n]), 0
}

func (f *blobFile) Release(_ context.Context) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rc != nil {
		_ = f.rc.Close()
		f.rc = nil
	}
	return 0
}

func (f *blobFile) Getattr(_ context.Context, out *fuse.AttrOut) syscall.Errno {
	out.Size = uint64(f.size)
	return 0
}
