package imager

import (
	"strings"
	"testing"

	. "github.com/bakape/meguca/test"
	"github.com/bakape/meguca/types"
)

func TestDetectArchive(t *testing.T) {
	t.Parallel()

	cases := [...]struct {
		name, file, err string
		typ             uint8
	}{
		{
			name: "ZIP",
			file: "sample.zip",
			typ:  types.ZIP,
		},
		{
			name: "7zip",
			file: "sample.7z",
			typ:  types.SevenZip,
		},
		{
			name: "tar.gz",
			file: "sample.tar.gz",
			typ:  types.TGZ,
		},
		{
			name: "tar.xz",
			file: "sample.tar.xz",
			typ:  types.TXZ,
		},
		{
			name: "file too small",
			file: "sample.txt",
			err:  "unsupported file type:",
		},
	}

	for i := range cases {
		c := cases[i]
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			typ, err := detectFileType(readSample(t, c.file))

			if c.err != "" {
				if err == nil {
					t.Fatalf("expected an error")
				}
				if !strings.HasPrefix(err.Error(), c.err) {
					t.Fatalf("unexpected error: %#v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}

			if typ != c.typ {
				t.Errorf("unexpected type: %d : %d", c.typ, typ)
			}
		})
	}
}

func TestProcessArchive(t *testing.T) {
	res := processArchive()
	if res.err != nil {
		t.Fatal(res.err)
	}

	AssertBufferEquals(t, res.thumb, readFallbackThumb(t, "archive-thumb.png"))
	assertDims(t, res.dims, [4]uint16{150, 150, 150, 150})
}
