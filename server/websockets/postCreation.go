package websockets

import (
	"bytes"
	"errors"
	"strings"
	"time"

	"unicode/utf8"

	"github.com/bakape/meguca/auth"
	"github.com/bakape/meguca/config"
	"github.com/bakape/meguca/db"
	"github.com/bakape/meguca/parser"
	"github.com/bakape/meguca/types"
	r "github.com/dancannon/gorethink"
)

var (
	errReadOnly          = errors.New("read only board")
	errInvalidImageToken = errors.New("invalid image token")
	errNoImageName       = errors.New("no image name")
	errImageNameTooLong  = errors.New("image name too long")
	errNoTextOrImage     = errors.New("no text or image")
	errThreadIsLocked    = errors.New("thread is locked")
)

// Websocket message response codes
const (
	postCreated = iota
	invalidInsertionCaptcha
)

type threadCreationRequest struct {
	postCreationCommon
	Subject, Board string
	types.Captcha
}

type postCreationCommon struct {
	Image                       imageRequest
	Name, Email, Auth, Password string
}

type imageRequest struct {
	Spoiler     bool
	Token, Name string
}

type replyCreationRequest struct {
	postCreationCommon
	Body string
}

// Response to a thread creation request
type threadCreationResponse struct {
	Code int   `json:"code"`
	ID   int64 `json:"id"`
}

// Insert a new thread into the database
func insertThread(data []byte, c *Client) (err error) {
	if err := closePreviousPost(c); err != nil {
		return err
	}
	var req threadCreationRequest
	if err := decodeMessage(data, &req); err != nil {
		return err
	}
	if !auth.IsNonMetaBoard(req.Board) {
		return errInvalidBoard
	}
	if !authenticateCaptcha(req.Captcha, c.IP) {
		return c.sendMessage(MessageInsertThread, threadCreationResponse{
			Code: invalidInsertionCaptcha,
		})
	}

	conf, err := getBoardConfig(req.Board)
	if err != nil {
		return err
	}

	post, now, err := constructPost(req.postCreationCommon, conf.ForcedAnon, c)
	if err != nil {
		return err
	}
	post.Board = req.Board
	thread := types.DatabaseThread{
		ReplyTime: now,
		Board:     req.Board,
	}
	thread.Subject, err = parser.ParseSubject(req.Subject)
	if err != nil {
		return err
	}

	// Perform this last, so there are less dangling images because of an error
	hasImage := !conf.TextOnly && req.Image.Token != "" && req.Image.Name != ""
	if hasImage {
		img := req.Image
		post.Image, err = getImage(img.Token, img.Name, img.Spoiler)
		thread.ImageCtr = 1
		if err != nil {
			return err
		}
	}

	id, err := db.ReservePostID()
	if err != nil {
		return err
	}
	thread.ID = id
	post.ID = id
	post.OP = id

	if err := db.Insert("posts", post); err != nil {
		return err
	}
	if err := db.Insert("threads", thread); err != nil {
		return err
	}
	if err := db.IncrementBoardCounter(req.Board); err != nil {
		return err
	}

	c.openPost = openPost{
		id:       id,
		op:       id,
		time:     now,
		board:    req.Board,
		hasImage: hasImage,
	}

	msg := threadCreationResponse{
		Code: postCreated,
		ID:   id,
	}
	return c.sendMessage(MessageInsertThread, msg)
}

// Insert a new post into the database
func insertPost(data []byte, c *Client) error {
	if err := closePreviousPost(c); err != nil {
		return err
	}

	var req replyCreationRequest
	if err := decodeMessage(data, &req); err != nil {
		return err
	}

	_, sync := Clients.GetSync(c)
	conf, err := getBoardConfig(sync.Board)
	if err != nil {
		return err
	}

	// Post must have either at least one character or an image to be allocated
	hasImage := !conf.TextOnly && req.Image.Token != "" && req.Image.Name != ""
	if req.Body == "" && !hasImage {
		return errNoTextOrImage
	}

	// Check thread is not locked and retrieve the post counter
	var threadAttrs struct {
		Locked  bool
		PostCtr int
	}
	q := r.Table("threads").Get(sync.OP).Pluck("locked", "postCtr")
	if err := db.One(q, &threadAttrs); err != nil {
		return err
	}
	if threadAttrs.Locked {
		return errThreadIsLocked
	}

	post, now, err := constructPost(req.postCreationCommon, conf.ForcedAnon, c)
	if err != nil {
		return err
	}

	// If the post contains a newline, slice till it and commit the remainder
	// separately as a splice
	var forSplicing string
	iNewline := strings.IndexRune(req.Body, '\n')
	if iNewline > -1 {
		forSplicing = req.Body[iNewline+1:]
		req.Body = req.Body[:iNewline]
	}
	post.Body = req.Body

	post.OP = sync.OP
	post.Board = sync.Board
	post.ID, err = db.ReservePostID()
	if err != nil {
		return err
	}

	updates := make(map[string]interface{}, 3)
	updates["postCtr"] = r.Row.Field("postCtr").Add(1)
	updates["replyTime"] = now

	if hasImage {
		img := req.Image
		post.Image, err = getImage(img.Token, img.Name, img.Spoiler)
		if err != nil {
			return err
		}
		updates["imageCtr"] = r.Row.Field("imageCtr").Add(1)
	}

	// Ensure the client knows the post ID before the public post insertion
	// update message is sent
	if err := c.sendMessage(MessagePostID, post.ID); err != nil {
		return err
	}

	if err := db.Insert("posts", post); err != nil {
		return err
	}
	q = r.Table("threads").Get(sync.OP).Update(updates)
	if err := db.Write(q); err != nil {
		return err
	}
	if err := db.IncrementBoardCounter(sync.Board); err != nil {
		return err
	}

	c.openPost = openPost{
		id:         post.ID,
		op:         sync.OP,
		time:       now,
		board:      sync.Board,
		Buffer:     *bytes.NewBuffer([]byte(post.Body)),
		bodyLength: utf8.RuneCountInString(post.Body),
		hasImage:   hasImage,
	}

	if forSplicing != "" {
		if err := parseLine(c, true); err != nil {
			return err
		}
		return spliceLine(spliceRequest{Text: []rune(forSplicing)}, c)
	}

	return nil
}

// If the client has a previous post, close it silently
func closePreviousPost(c *Client) error {
	if c.openPost.id != 0 {
		return closePost(nil, c)
	}
	return nil
}

// Retrieve post-related board configurations
func getBoardConfig(board string) (conf config.PostParseConfigs, err error) {
	conf = config.GetBoardConfigs(board).PostParseConfigs
	if conf.ReadOnly {
		err = errReadOnly
	}
	return
}

// Construct the common parts of the new post for both threads and replies
func constructPost(req postCreationCommon, forcedAnon bool, c *Client) (
	post types.DatabasePost, now int64, err error,
) {
	now = time.Now().Unix()
	post = types.DatabasePost{
		StandalonePost: types.StandalonePost{
			Post: types.Post{
				Editing: true,
				Time:    now,
				Email:   parser.FormatEmail(req.Email),
			},
		},
		LastUpdated: now,
		IP:          c.IP,
	}
	if !forcedAnon {
		post.Name, post.Trip, err = parser.ParseName(req.Name)
		if err != nil {
			return
		}
	}
	err = parser.VerifyPostPassword(req.Password)
	if err != nil {
		return
	}
	post.Password, err = auth.BcryptHash(req.Password, 6)

	// TODO: Staff title verification

	return
}

// Performs some validations and retrieves processed image data by token ID.
// Embeds spoiler and image name in result struct. The last extension is
// stripped from the name.
func getImage(token, name string, spoiler bool) (*types.Image, error) {
	switch {
	case len(token) > 127: // RethinkDB key length limit
		return nil, errInvalidImageToken
	case len(name) > 200:
		return nil, errImageNameTooLong
	}

	imgCommon, err := db.UseImageToken(token)
	switch err {
	case nil:
	case db.ErrInvalidToken:
		return nil, errInvalidImageToken
	default:
		return nil, err
	}

	// Trim on the first dot in the file name. Not using filepath.Ext(), because
	// it does not handle compound extensions like ".tar.gz"
	if i := strings.IndexByte(name, '.'); i != -1 {
		name = name[:i]
	}

	return &types.Image{
		ImageCommon: imgCommon,
		Spoiler:     spoiler,
		Name:        name,
	}, nil
}
