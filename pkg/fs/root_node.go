// Ported from stargz-snapshotter, copyright The stargz-snapshotter Authors.
// https://github.com/containerd/stargz-snapshotter/blob/efc4166e93a22804b90e27c912eff7ecc0a12dfc/store/fs.go#L159-L221
package fs

import (
	"context"
	"encoding/base64"
	"log/slog"
	"syscall"

	"github.com/containerd/containerd/reference"
	fusefs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// rootnode is the mountpoint node of nydus-store.
type rootNode struct {
	fusefs.Inode
	fs *fs
}

var _ = (fusefs.InodeEmbedder)((*rootNode)(nil))

var _ = (fusefs.NodeLookuper)((*rootNode)(nil))
var _ = (fusefs.NodeReaddirer)((*rootNode)(nil))
var _ = (fusefs.NodeUnlinker)((*rootNode)(nil))

// Lookup loads manifest and config of specified name (image reference)
// and returns refnode of the specified name
func (n *rootNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fusefs.Inode, syscall.Errno) {
	slog.InfoContext(ctx, "root node lookup", "name", name)
	if child := n.GetChild(name); child != nil {
		switch tn := child.Operations().(type) {
		case *fusefs.MemSymlink:
			copyAttr(&out.Attr, &tn.Attr)
		case *refNode:
			copyAttr(&out.Attr, &tn.attr)
		default:
			slog.WarnContext(ctx, "rootNode.Lookup: unknown node type detected")
			return nil, syscall.EIO
		}
		out.Ino = child.StableAttr().Ino
		return child, 0
	}

	switch name {
	case poolLink:
		// filesystem cache.
		sAttr := defaultLinkAttr(&out.Attr)
		cn := &fusefs.MemSymlink{Data: []byte(n.fs.layManager.RefRoot())}
		copyAttr(&cn.Attr, &out.Attr)
		return n.fs.newInodeWithID(ctx, func(ino uint32) fusefs.InodeEmbedder {
			out.Ino = uint64(ino)
			cn.Attr.Ino = uint64(ino)
			sAttr.Ino = uint64(ino)
			return n.NewInode(ctx, cn, sAttr)
		})
	case "test-nydus-store-alive":
		// Test file to verify Podman is accessing our FUSE mount
		slog.InfoContext(ctx, "TEST: Podman accessed our test file!", "name", name)
		sAttr := defaultFileAttr(uint64(len("nydus-storage-plugin-is-alive")), &out.Attr)
		cn := &fusefs.MemRegularFile{
			Data: []byte("nydus-storage-plugin-is-alive"),
		}
		copyAttr(&cn.Attr, &out.Attr)
		return n.fs.newInodeWithID(ctx, func(ino uint32) fusefs.InodeEmbedder {
			out.Ino = uint64(ino)
			cn.Attr.Ino = uint64(ino)
			sAttr.Ino = uint64(ino)
			return n.NewInode(ctx, cn, sAttr)
		})
	}

	refBytes, err := base64.StdEncoding.DecodeString(name)
	if err != nil {
		slog.ErrorContext(ctx, "failed to decode ref base64", "name", name, "err", err)
		return nil, syscall.EINVAL
	}
	ref := string(refBytes)
	var refSpec reference.Spec
	refSpec, err = reference.Parse(ref)
	if err != nil {
		slog.ErrorContext(ctx, "invalid reference", "ref", ref, "raw", name, "err", err)
		return nil, syscall.EINVAL
	}
	sAttr := defaultDirAttr(&out.Attr)
	child := &refNode{
		fs:     n.fs,
		ref:    refSpec,
		rawRef: name,
	}
	copyAttr(&child.attr, &out.Attr)
	return n.fs.newInodeWithID(ctx, func(ino uint32) fusefs.InodeEmbedder {
		out.Ino = uint64(ino)
		child.attr.Ino = uint64(ino)
		sAttr.Ino = uint64(ino)
		return n.NewInode(ctx, child, sAttr)
	})
}

// Readdir enumerates entries in the root directory.
// Shows the "pool" symlink and any cached image references.
func (n *rootNode) Readdir(ctx context.Context) (fusefs.DirStream, syscall.Errno) {
	// Start with the pool symlink
	entries := []fuse.DirEntry{
		{Name: poolLink, Mode: fuse.S_IFLNK},
		{Name: "test-nydus-store-alive", Mode: fuse.S_IFREG},
	}

	// TODO: Add existing image references
	// Currently, we only show the pool symlink since that's what's always available

	return fusefs.NewListDirStream(entries), 0
}

// Unlink prevents deletion of critical system entries like "pool"
func (n *rootNode) Unlink(ctx context.Context, name string) syscall.Errno {
	slog.InfoContext(ctx, "root node unlink attempt", "name", name)

	// Prevent deletion of the pool symlink
	if name == poolLink {
		slog.WarnContext(ctx, "attempted to delete protected pool symlink", "name", name)
		return syscall.EPERM // Operation not permitted
	}

	// For other files, we don't support deletion
	slog.InfoContext(ctx, "unlink not supported", "name", name)
	return syscall.EPERM
}
