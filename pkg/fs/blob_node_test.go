package fs

import (
	"context"
	"testing"
)

func TestBlobNodeOpenReturnsBlobFile(t *testing.T) {
	n := &blobNode{}
	fh, _, eno := n.Open(context.Background(), 0)
	if eno != 0 {
		t.Fatalf("Open errno=%d", eno)
	}
	if _, ok := fh.(*blobFile); !ok {
		t.Fatalf("expected *blobFile, got %T", fh)
	}
}
