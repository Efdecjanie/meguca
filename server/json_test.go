package server

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/bakape/meguca/auth"
	"github.com/bakape/meguca/config"
	"github.com/bakape/meguca/db"
	. "github.com/bakape/meguca/test"
	"github.com/bakape/meguca/types"
	r "github.com/dancannon/gorethink"
)

var genericImage = &types.Image{
	ImageCommon: types.ImageCommon{
		SHA1: "foo",
	},
}

func removeIndentation(s string) string {
	s = strings.Replace(s, "\t", "", -1)
	s = strings.Replace(s, "\n", "", -1)
	return s
}

func TestServeConfigs(t *testing.T) {
	etag := "foo"
	config.SetClient([]byte{1}, etag)

	rec, req := newPair("/json/config")
	router.ServeHTTP(rec, req)
	assertCode(t, rec, 200)
	assertBody(t, rec, string([]byte{1}))
	assertEtag(t, rec, etag)

	// And with etag
	rec, req = newPair("/json/config")
	req.Header.Set("If-None-Match", etag)
	router.ServeHTTP(rec, req)
	assertCode(t, rec, 304)
}

func TestDetectLastN(t *testing.T) {
	t.Parallel()

	cases := [...]struct {
		name, in string
		out      int
	}{
		{"no query string", "/a/1", 0},
		{"unparsable", "/a/1?last=addsa", 0},
		{"5", "/a/1?last=5", 5},
		{"50", "/a/1?last=50", 50},
		{"invalid number", "/a/1?last=1000", 0},
	}

	for i := range cases {
		c := cases[i]
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			req := newRequest(c.in)
			if n := detectLastN(req); n != c.out {
				LogUnexpected(t, c.out, n)
			}
		})
	}
}

func TestPostJSON(t *testing.T) {
	assertTableClear(t, "main", "posts", "threads")
	assertInsert(t, "main", []map[string]interface{}{
		{
			"id":      "info",
			"postCtr": 8,
		},
		{
			"id": "boardCtrs",
			"a":  7,
		},
	})
	assertInsert(t, "threads", types.DatabaseThread{
		ID:    1,
		Board: "a",
	})
	assertInsert(t, "posts", types.DatabasePost{
		StandalonePost: types.StandalonePost{
			Post: types.Post{
				ID: 1,
			},
			Board: "a",
			OP:    1,
		},
		LastUpdated: 11,
	})

	setBoards(t, "a")

	const postEtag = "qO18VR0TvaL71iNdrFmaIQ"

	cases := [...]struct {
		name, url, header string
		code              int
		etag              string
	}{
		{
			"invalid post number",
			"/post/www",
			"", 400, "",
		},
		{
			"nonexistent post",
			"/post/66",
			"", 404, "",
		},
		{
			"existing post",
			"/post/1",
			"", 200, postEtag,
		},
		{
			"post etag matches",
			"/post/1",
			postEtag, 304, "",
		},
		{
			"invalid thread board",
			"/nope/1",
			"", 404, "",
		},
		{
			"invalid thread number",
			"/a/www",
			"", 400, "",
		},
		{
			"nonexistent thread",
			"/a/22",
			"", 404, "",
		},
		{
			"valid thread",
			"/a/1",
			"", 200, "W/11",
		},
		{
			"thread etags match",
			"/a/1",
			"W/11", 304, "",
		},
		{
			"invalid board",
			"/nope/",
			"", 404, "",
		},
		{
			"valid board",
			"/a/",
			"", 200, "W/7",
		},
		{
			"board etag matches",
			"/a/",
			"W/7", 304, "",
		},
		{
			"all board",
			"/all/",
			"", 200, "W/8",
		},
		{
			"/all/ board etag matches",
			"/all/",
			"W/8", 304, "",
		},
	}

	for i := range cases {
		c := cases[i]
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			rec, req := newPair("/json" + c.url)
			if c.header != "" {
				req.Header.Set("If-None-Match", c.header)
			}
			router.ServeHTTP(rec, req)
			assertCode(t, rec, c.code)
			if c.code == 200 {
				assertEtag(t, rec, c.etag)
			}
		})
	}
}

func TestServeBoardConfigs(t *testing.T) {
	setBoards(t, "a")
	config.AllBoardConfigs.JSON = []byte("foo")
	conf := config.BoardConfigs{
		ID: "a",
		BoardPublic: config.BoardPublic{
			CodeTags: true,
			Title:    "Animu",
			Notice:   "Notice",
			Banners:  []string{},
		},
	}
	config.SetBoardConfigs(conf)

	cases := [...]struct {
		name, url string
		code      int
		body      string
	}{
		{"invalid board", "aaa", 404, ""},
		{"valid board", "a", 200, string(marshalJSON(t, conf.BoardPublic))},
		{"/all/ board", "all", 200, "foo"},
	}

	for i := range cases {
		c := cases[i]
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			rec, req := newPair("/json/boardConfig/" + c.url)
			router.ServeHTTP(rec, req)
			assertCode(t, rec, c.code)
			if c.code == 200 {
				assertBody(t, rec, c.body)
			}
		})
	}
}

func TestServeBoardList(t *testing.T) {
	assertTableClear(t, "boards")

	// No boards
	rec, req := newPair("/json/boardList")
	router.ServeHTTP(rec, req)
	assertBody(t, rec, "[]")

	assertInsert(t, "boards", []config.BoardConfigs{
		{
			ID: "a",
			BoardPublic: config.BoardPublic{
				Title: "Animu",
			},
		},
		{
			ID: "g",
			BoardPublic: config.BoardPublic{
				Title: "Technology",
			},
		},
	})

	std := removeIndentation(`
[
	{
		"id":"a",
		"title":"Animu"
	},
	{
		"id":"g",
		"title":"Technology"
	}
]`)

	rec, req = newPair("/json/boardList")
	router.ServeHTTP(rec, req)
	assertBody(t, rec, std)
}

func TestServeStaffPosition(t *testing.T) {
	assertTableClear(t, "boards")
	staff := map[string][]string{
		"owners": {"admin"},
	}
	assertInsert(t, "boards", []config.BoardConfigs{
		{
			ID:    "a",
			Staff: staff,
		},
		{
			ID: "b",
		},
		{
			ID:    "c",
			Staff: staff,
		},
	})

	cases := [...]struct {
		name, position, user, res string
	}{
		{"valid query", "owners", "admin", `["a","c"]`},
		{"invalid user", "mod", "admin", "[]"},
		{"invalid position", "owners", "bullshit", "[]"},
		{"both invalid", "bullocks", "bullshit", "[]"},
	}

	for i := range cases {
		c := cases[i]
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			path := fmt.Sprintf("/json/positions/%s/%s", c.position, c.user)
			rec, req := newPair(path)
			router.ServeHTTP(rec, req)
			assertCode(t, rec, 200)
			assertBody(t, rec, c.res)
		})
	}
}

func TestSpoilerImage(t *testing.T) {
	assertTableClear(t, "posts")

	const password = "123"
	hash, err := auth.BcryptHash(password, 6)
	if err != nil {
		t.Fatal(err)
	}

	assertInsert(t, "posts", []types.DatabasePost{
		{
			Password: hash,
			StandalonePost: types.StandalonePost{
				Post: types.Post{
					ID: 1,
					Image: &types.Image{
						ImageCommon: types.ImageCommon{
							SHA1: "123",
						},
					},
				},
			},
		},
		{
			Password: hash,
			StandalonePost: types.StandalonePost{
				Post: types.Post{
					ID: 2,
				},
			},
		},
		{
			Password: hash,
			StandalonePost: types.StandalonePost{
				Post: types.Post{
					ID: 3,
					Image: &types.Image{
						ImageCommon: types.ImageCommon{
							SHA1: "123",
						},
						Spoiler: true,
					},
				},
			},
		},
		{
			Password: hash,
			StandalonePost: types.StandalonePost{
				Post: types.Post{
					ID: 4,
					Image: &types.Image{
						ImageCommon: types.ImageCommon{
							SHA1: "123",
						},
					},
				},
			},
		},
	})

	cases := [...]struct {
		name      string
		id        int64
		password  string
		code      int
		spoilered bool
	}{
		{"no image", 2, password, 400, false},
		{"wrong password", 4, "122", 403, false},
		{"success", 1, password, 200, true},
		{"already spoilered", 3, password, 200, true},
	}

	for i := range cases {
		c := cases[i]
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			data := spoilerRequest{
				ID:       c.id,
				Password: c.password,
			}
			rec, req := newJSONPair(t, "/json/spoiler", data)
			router.ServeHTTP(rec, req)

			assertCode(t, rec, c.code)

			var spoilered bool
			msg := []byte("11" + strconv.Itoa(int(c.id)))
			post := db.FindPost(c.id)
			q := r.And(
				post.Field("log").Contains(msg),
				post.Field("image").Field("spoiler"),
			)
			if err := db.One(q, &spoilered); err != nil {
				t.Fatal(err)
			}
			if spoilered != spoilered {
				t.Errorf(
					"spoiler mismatch: expected %v; got %v",
					c.spoilered,
					spoilered,
				)
			}
		})
	}
}

func TestServeBoardTimeStamps(t *testing.T) {
	setBoards(t, "a", "c")
	assertTableClear(t, "posts")
	assertInsert(t, "posts", []types.DatabasePost{
		{
			LastUpdated: 1,
			StandalonePost: types.StandalonePost{
				Board: "a",
				Post: types.Post{
					ID: 11,
				},
			},
		},
		{
			LastUpdated: 2,
			StandalonePost: types.StandalonePost{
				Board: "a",
				Post: types.Post{
					ID: 22,
				},
			},
		},
		{
			LastUpdated: 3,
			StandalonePost: types.StandalonePost{
				Board: "c",
				Post: types.Post{
					ID: 33,
				},
			},
		},
	})

	rec, req := newPair("/json/boardTimestamps")
	router.ServeHTTP(rec, req)
	assertBody(t, rec, `{"a":2,"c":3}`)
}

func TestServeExtensionMap(t *testing.T) {
	t.Parallel()
	rec, req := newPair("/json/extensions")
	router.ServeHTTP(rec, req)
	assertCode(t, rec, 200)
}
