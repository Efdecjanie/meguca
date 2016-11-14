package templates

import (
	"testing"

	"github.com/bakape/meguca/imager/assets"
	"github.com/bakape/meguca/lang"
	"github.com/bakape/meguca/types"
)

func TestBoard(t *testing.T) {
	_, err := Board("all", lang.Packs["en_GB"], true, types.Board{
		Threads: types.BoardThreads{
			{
				ID:      1,
				Board:   "a",
				Subject: "foo",
			},
			{
				ID:      2,
				Board:   "c",
				Subject: "bar",
				Image:   &assets.StdJPEG,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}
