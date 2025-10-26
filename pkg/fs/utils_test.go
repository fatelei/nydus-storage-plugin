package fs

import (
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestUtilsCopyAttr(t *testing.T) {
	var src, dst fuse.Attr
	src.Ino = 123
	src.Size = 456
	src.Mode = 0o644
	src.Nlink = 2
	src.Owner = fuse.Owner{Uid: 1, Gid: 2}
	copyAttr(&dst, &src)
	if dst != src {
		t.Fatalf("attr not copied: %+v != %+v", dst, src)
	}
}

func TestUtilsLayerToAttr(t *testing.T) {
	var out fuse.Attr
	s := layerToAttr(&ocispec.Descriptor{Size: 8192}, &out)
	if out.Mode != layerFileMode {
		t.Fatalf("unexpected mode: %o", out.Mode)
	}
	if s.Mode != out.Mode {
		t.Fatalf("stable mode mismatch")
	}
}

func TestUtilsDefaultFileDirLinkAttr(t *testing.T) {
	var out fuse.Attr
	s1 := defaultFileAttr(100, &out)
	if out.Mode != defaultFileMode || s1.Mode != out.Mode {
		t.Fatalf("defaultFileAttr unexpected")
	}
	s2 := defaultDirAttr(&out)
	if out.Mode != defaultDirMode || s2.Mode != out.Mode {
		t.Fatalf("defaultDirAttr unexpected")
	}
	s3 := defaultLinkAttr(&out)
	if out.Mode != defaultLinkMode || s3.Mode != out.Mode {
		t.Fatalf("defaultLinkAttr unexpected")
	}
}
