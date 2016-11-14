package server

import (
	"testing"

	"github.com/bakape/meguca/config"
	"github.com/bakape/meguca/types"
)

func TestThreadHTML(t *testing.T) {
	assertTableClear(t, "threads")
	assertInsert(t, "threads", types.DatabaseThread{
		ID:    1,
		Board: "a",
	})
	setBoards(t, "a")

	t.Run("unparsable thread number", func(t *testing.T) {
		t.Parallel()

		rec, req := newPair("/a/www")
		router.ServeHTTP(rec, req)
		assertCode(t, rec, 400)
	})
	t.Run("nonexistent thread", func(t *testing.T) {
		t.Parallel()

		rec, req := newPair("/a/22")
		router.ServeHTTP(rec, req)
		assertCode(t, rec, 404)
	})
	t.Run("thread exists", func(t *testing.T) {
		t.Parallel()

		rec, req := newPair("/a/1")
		router.ServeHTTP(rec, req)
		assertCode(t, rec, 200)
	})
}

func TestBoardHTML(t *testing.T) {
	setupPosts(t)
	setBoards(t, "a")
	(*config.Get()).DefaultLang = "en_GB"

	cases := [...]struct {
		name, url string
		code      int
	}{
		{"/all/ board", "/all/", 200},
		{"regular board", "/a/", 200},
		{"without index template", "/a/?noIndex=true", 200},
		{"non-existent board", "/b/", 404},
	}

	for i := range cases {
		c := cases[i]
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			rec, req := newPair(c.url)
			router.ServeHTTP(rec, req)
			assertCode(t, rec, c.code)
		})
	}
}

func TestBoardNavigation(t *testing.T) {
	(*config.Get()).DefaultLang = "en_GB"

	rec, req := newPair("/forms/boardNavigation")
	router.ServeHTTP(rec, req)
	assertCode(t, rec, 200)
}
