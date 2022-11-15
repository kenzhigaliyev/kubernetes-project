package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func register(w http.ResponseWriter, r *http.Request) {

	// Struct request body
	var reg struct {
		Username string `json:"username"`
		Fullname string `json:"fullname"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	readBody(r, &reg)

	errcheck := func(e error, ex string) bool {
		return strings.Contains(e.Error(), ex)
	}

	// Create new validation report
	type report struct {
		UsernameFormat bool
		FullnameFormat bool
		EmailFormat    bool
		PasswordFormat bool
		EmailExist     bool
		UsernameExist  bool
	}
	var valid report

	valid.UsernameFormat = regcheck(reg.Username, `^[a-zA-Z0-9_]{3,10}$`)
	valid.EmailFormat = regcheck(reg.Email, `^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
	valid.FullnameFormat = regcheck(reg.Fullname, `^.{3,20}$`)
	valid.PasswordFormat = regcheck(reg.Password, `^.{6,}$`)

	// Encrypt password for safe storage
	pass := encrypt(reg.Password)

	// Making conditional insert - if data is valid insert will be commited
	query := `INSERT INTO users(email, username, fullname, password) values( $1, $2, $3, $4)`
	rollback := insert(query, (report{}) != valid, reg.Email, reg.Username, reg.Fullname, pass)

	// If conditional insert returned error - insert will rollback
	if rollback != nil {

		// Validate Email and Username for case of uniqness in DB
		valid.EmailExist = errcheck(rollback, "users.email")
		valid.UsernameExist = errcheck(rollback, "users.user")

		// Return Error with validation report
		w.WriteHeader(400)
		returnJSON(valid, w)
		return
	}

}

func login(w http.ResponseWriter, r *http.Request) {

	// Struct request body
	var login struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	readBody(r, &login)

	fmt.Println(login)

	// Get encrypted password from DB and user ID
	var creds []struct {
		Username string
		Fullname string
		Password string
		UserID   int64
		Role     string
	}

	query := `SELECT username, fullname, password, userId, role FROM users WHERE username = $1`
	sliceFromDB(&creds, query, nil, login.Username)

	// If no such user in DB or password is wrong
	if len(creds) == 0 || !cryptIsValid(creds[0].Password, login.Password) {
		http.Error(w, http.StatusText(403), 403)
		return
	}

	// Set new JWT if password correct
	setJWT(creds[0].UserID, creds[0].Role, w)

	// Return creds as json
	var user struct {
		UserID   int64  `json:"userID"`
		Username string `json:"username"`
		Fullname string `json:"fullname"`
	}
	user.UserID = creds[0].UserID
	user.Username = creds[0].Username
	user.Fullname = creds[0].Fullname
	returnJSON(user, w)

}

func logout(w http.ResponseWriter, r *http.Request) {
	delete(sessions, ctx("user", r).(ctxData).ID)
	addCookie(w, "jwt", "", time.Unix(0, 0))
}

func writepost(w http.ResponseWriter, r *http.Request) {

	// Struct request body
	var post struct {
		Title      string  `json:"title"`
		Text       string  `json:"text"`
		Categories []int64 `json:"categories"`
		PostID     int64   `json:"postID"`
		Status     int64   `json:"status"`
	}
	readBody(r, &post)

	role := ctx("user", r).(ctxData).Role
	uid := ctx("user", r).(ctxData).ID

	var e error
	if post.Status == 0 && post.Title == "" && post.PostID != 0 {
		upd := `UPDATE posts SET status = $1 WHERE postId = $2 AND (userId = $3 OR $4 = 'admin' OR $4 = 'moderator')`
		e = insert(upd, false, post.Status, post.PostID, uid, role)
		err(e)
		return
	}

	// Create validation report
	type report struct {
		Title         bool
		Status        bool
		CategoriesNum bool
		CategoriesVal bool
	}

	var validity report
	validity.Title = regcheck(strings.TrimSpace(post.Title), `^.{3,140}$`)
	validity.Status = (post.Status > 2 || post.Status < 0) && post.PostID != 0
	validity.CategoriesNum = len(post.Categories) > 3 || len(post.Categories) == 0

	if (report{}) != validity {
		w.WriteHeader(400)
		returnJSON(validity, w)
		return
	}

	cats, catsError := processCategories(post.Categories)
	if catsError != nil {
		validity.CategoriesVal = true
		w.WriteHeader(400)
		returnJSON(validity, w)
		return
	}

	text := ""
	if strings.TrimSpace(post.Text) != "" {
		text = post.Text
	}

	if post.PostID == 0 {
		ins := `INSERT INTO posts(title, text, categories, userId) values($1, $2, $3, $4)`
		e = insert(ins, false, post.Title, text, cats, uid)
	} else {
		upd := `UPDATE posts SET 
			title = $1, 
			text = $2, 
			categories = $3,
			status = $4 
			WHERE postId = $5 AND (userId = $6 OR $7 = 'admin' OR $7 = 'moderator')`
		e = insert(upd, false, post.Title, text, cats, post.Status, post.PostID, uid, role)
	}
	err(e)
}

func posts(w http.ResponseWriter, r *http.Request) {

	// Get request query params
	cat := "%\"" + reqQuery("cat", r) + "\"%"
	userID := reqQuery("userID", r)
	search := "%" + reqQuery("search", r) + "%"
	status := reqQuery("status", r)
	postID := reqQuery("postID", r)
	byreact := r.FormValue("byreact")

	order := "ORDER BY p.posted DESC"
	if byreact == "likes" || byreact == "dislikes" {
		order = "ORDER BY " + byreact + " DESC"
	}

	// Pagination params
	pageSize := 100
	page, atoiError := strconv.Atoi(r.FormValue("page"))
	if atoiError != nil || page <= 0 {
		page = 1
	}
	offset := page*pageSize - pageSize

	// Select RAW slice data from DB
	var postDB []struct {
		PostID     int64         `json:"pid"`
		Posted     int64         `json:"created"`
		AuthorID   int64         `json:"uid"`
		Username   string        `json:"username"`
		Title      string        `json:"text"`
		Likes      int64         `json:"likes"`
		Dislikes   int64         `json:"dislikes"`
		Comments   int64         `json:"comments"`
		Reaction   string        `json:"reaction"`
		Categories []interface{} `json:"categories"`
	}
	query := `
	SELECT 
		p.postId,
		CAST(strftime('%s', p.posted) AS INT),
		p.userId,
		(SELECT username FROM users u WHERE u.userId = p.userId),
		p.title, 
		(SELECT COUNT(*) FROM postReactions r WHERE r.postId = p.postId AND reaction = 'like') AS likes,
		(SELECT COUNT(*) FROM postReactions r WHERE r.postId = p.postId AND reaction = 'dislike') AS dislikes,
		(SELECT COUNT(*) FROM comments c WHERE c.postId = p.postId) AS Comments,
		COALESCE((SELECT reaction FROM postReactions r WHERE r.postId = p.postId AND r.userId = $1), "idle"),
		p.categories
	FROM posts p WHERE 
	p.status > '0'
	AND p.categories LIKE $2 
	AND p.userId LIKE $3 
	AND p.title LIKE $4 
	AND p.postId LIKE $5
	AND p.status LIKE $6 ` + order + ` LIMIT $7 OFFSET $8`

	uid := ctx("user", r).(ctxData).ID
	sliceFromDB(&postDB, query, getCats, uid, cat, userID, search, postID, status, pageSize, offset)
	returnJSON(postDB, w)
}

func post(w http.ResponseWriter, r *http.Request) {

	// Get request query params
	postID := r.FormValue("postID")

	// Select RAW slice data from DB
	var postDB []struct {
		PostID     int64
		AuthorID   int64
		Username   string
		Title      string
		Text       string
		Reaction   string
		Likes      int64
		Dislikes   int64
		Categories []interface{}
	}
	query := `
	SELECT 
		p.postId,
		p.userId,
		(SELECT username FROM users u WHERE u.userId = p.userId),
		p.title, 
		p.text,
		COALESCE((SELECT reaction FROM postReactions r WHERE r.postId = p.postId AND r.userId = $1), "idle"),
		(SELECT COUNT(*) FROM postReactions r WHERE r.postId = p.postId AND reaction = 'like') AS likes,
		(SELECT COUNT(*) FROM postReactions r WHERE r.postId = p.postId AND reaction = 'dislike') AS dislikes,
		p.categories
	FROM posts p WHERE p.status > '0' AND p.postId = $2`
	uid := ctx("user", r).(ctxData).ID
	sliceFromDB(&postDB, query, getCats, uid, postID)
	returnJSON(postDB[0], w)

}

func comments(w http.ResponseWriter, r *http.Request) {
	uid := ctx("user", r).(ctxData).ID
	var comments []struct {
		CommentID int64  `json:"cid"`
		Commented int64  `json:"created"`
		AuthorID  int64  `json:"uid"`
		Username  string `json:"username"`
		Comment   string `json:"text"`
		Like      int64  `json:"likes"`
		Dislike   int64  `json:"dislikes"`
		Reaction  string `json:"reaction"`
	}
	query := `
	SELECT 
		commentId,
		CAST(strftime('%s', commented) AS INT),
		c.userId,
		(SELECT username FROM users u WHERE u.userId = c.userId),
		comment,
		(SELECT COUNT(*) FROM commentReactions r WHERE r.commentId = c.commentId AND reaction = 'like'),
		(SELECT COUNT(*) FROM commentReactions r WHERE r.commentId = c.commentId AND reaction = 'dislike'),
		COALESCE((SELECT reaction FROM commentReactions r WHERE r.commentId = c.commentId AND r.userId = $1), "idle")
	FROM comments c
	WHERE c.status > '0' AND c.postId = $2 ORDER BY commented DESC`

	sliceFromDB(&comments, query, nil, uid, r.FormValue("postID"))
	returnJSON(comments, w)
}

func writecomment(w http.ResponseWriter, r *http.Request) {
	var comment struct {
		PostID    int64  `json:"postID"`
		Comment   string `json:"comment"`
		CommentID int64  `json:"commentID"`
		Status    int64  `json:"status"`
	}
	readBody(r, &comment)

	role := ctx("user", r).(ctxData).Role
	uid := ctx("user", r).(ctxData).ID
	if comment.Status == 0 && comment.Comment == "" {
		upd := `UPDATE comments SET status = $1 WHERE commentId= $2 AND (userId = $3 OR $4 = 'admin' OR $4 = 'moderator')`
		err(insert(upd, false, comment.Status, comment.CommentID, uid, role))
		return
	}

	if strings.TrimSpace(comment.Comment) == "" {
		w.WriteHeader(400)
		return
	}

	newComment := strings.ReplaceAll(comment.Comment, "\n", "<br>")

	if comment.CommentID == 0 {
		ins := `INSERT INTO comments(postId, comment, userId) VALUES ((SELECT postId FROM posts WHERE postId = $1), $2, $3)`
		err(insert(ins, false, comment.PostID, newComment, uid))
	} else {
		upd := `UPDATE comments SET comment = $1 WHERE commentId= $2 AND (userId = $3 OR $4 = 'admin' OR $4 = 'moderator')`
		err(insert(upd, false, newComment, comment.CommentID, uid, role))
	}
}

func reaction(w http.ResponseWriter, r *http.Request) {
	var react struct {
		PostID    int64  `json:"postID"`
		CommentID int64  `json:"commentID"`
		Reaction  string `json:"reaction"`
	}
	readBody(r, &react)
	var id int64
	var query string
	var upd string

	reactionValid := react.Reaction == "like" || react.Reaction == "dislike" || react.Reaction == "idle"
	if react.PostID > 0 && react.CommentID == 0 && reactionValid {
		id = react.PostID
		query = `INSERT INTO postReactions(reaction, postId, userId) VALUES ($1, (SELECT postId FROM posts WHERE postId = $2), $3)`
		upd = `UPDATE postReactions SET reaction = (CASE WHEN reaction = $1 THEN 'idle' ELSE $1 END) WHERE postId = $2 AND userId = $3`

	} else if react.PostID == 0 && react.CommentID > 0 && reactionValid {
		id = react.CommentID
		query = `INSERT INTO commentReactions(reaction, commentId, userId) VALUES ($1, (SELECT commentId FROM comments WHERE commentId = $2), $3)`
		upd = `UPDATE commentReactions SET reaction = (CASE WHEN reaction = $1 THEN 'idle' ELSE $1 END) WHERE commentId = $2 AND userId = $3`

	} else {
		http.Error(w, http.StatusText(400), 400)
		return
	}
	uid := ctx("user", r).(ctxData).ID
	rollback := insert(query, false, react.Reaction, id, uid)
	if rollback != nil {
		err(insert(upd, false, react.Reaction, id, uid))
	}

}

func updcategory(w http.ResponseWriter, r *http.Request) {
	if ctx("user", r).(ctxData).Role != "admin" {
		http.Error(w, http.StatusText(403), 403)
		return
	}
	var cat struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		CategoryID  int64  `json:"categoryID"`
	}
	readBody(r, &cat)
	var validity report
	validity.regcheck("wrong category name", cat.Name, `^.{2,10}$`)
	validity.regcheck("wrong description", cat.Description, `^.{2,}$`)
	if ctx("user", r).(ctxData).Role == "admin" && len(validity) == 0 {
		if cat.CategoryID == 0 {
			query := "INSERT INTO categories(name, description) VALUES ($1, $2)"
			err(insert(query, false, cat.Name, cat.Description))
		} else {
			query := "UPDATE categories SET name = $1, description = $2 WHERE categoryId = $3"
			err(insert(query, false, cat.Name, cat.Description, cat.CategoryID))
		}
	}
}

func deletecategory(w http.ResponseWriter, r *http.Request) {
	if ctx("user", r).(ctxData).Role != "admin" {
		http.Error(w, http.StatusText(403), 403)
		return
	}
	var cat struct {
		CategoryID int64 `json:"categoryID"`
	}
	readBody(r, &cat)
	category := "%\"" + strconv.FormatInt(cat.CategoryID, 10) + "\"%"
	query := `
		DELETE FROM categories WHERE categoryId = $1;
		UPDATE posts SET categories = REPLACE(categories, $2, '') WHERE categories LIKE $2;`
	err(insert(query, false, cat.CategoryID, category))
}

func categories(w http.ResponseWriter, r *http.Request) {
	var cat []struct {
		CategoryID  int64  `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	query := `SELECT categoryId, name, description FROM categories`
	sliceFromDB(&cat, query, nil)
	returnJSON(cat, w)
}

func users(w http.ResponseWriter, r *http.Request) {
	if ctx("user", r).(ctxData).Role != "admin" {
		http.Error(w, http.StatusText(403), 403)
		return
	}
	var users []struct {
		Fullname string
		Username string
		Email    string
		Role     string
		Status   int64
	}
	query := `SELECT fullname, username, email, role, status FROM users`
	sliceFromDB(&users, query, nil)
	returnJSON(users, w)
}

func changerole(w http.ResponseWriter, r *http.Request) {
	if ctx("user", r).(ctxData).Role != "admin" {
		http.Error(w, http.StatusText(403), 403)
		return
	}
	var user struct {
		UserID int64  `json:"userID"`
		Role   string `json:"role"`
	}
	readBody(r, &user)
	query := `UPDATE users SET role = $1 WHERE userId = $2`
	err(insert(query, false, user.Role, user.UserID))
}

func claim(w http.ResponseWriter, r *http.Request) {
	var claim struct {
		PostID    int64  `json:"postID"`
		CommentID int64  `json:"commentID"`
		Text      string `json:"text"`
	}
	readBody(r, &claim)

	var validity report
	validity.regcheck("no claim text", strings.TrimSpace(claim.Text), `^.{2,}$`)

	if len(validity) > 0 {
		w.WriteHeader(400)
		returnJSON(validity, w)
		return
	}

	var query string
	var id int64
	if claim.PostID > 0 && claim.CommentID == 0 {
		id = claim.PostID
		query = `INSERT INTO claims(type, textId, claim, userId) VALUES('post', $1, $2, $3)`
	} else if claim.PostID == 0 && claim.CommentID > 0 {
		id = claim.CommentID
		query = `INSERT INTO claims(type, textId, claim, userId) VALUES('comment', $1, $2, $3)`
	} else {
		http.Error(w, http.StatusText(400), 400)
		return
	}
	uid := ctx("user", r).(ctxData).ID
	err(insert(query, false, id, claim.Text, uid))
}

func viewclaims(w http.ResponseWriter, r *http.Request) {
	var claims []struct {
		ClaimID  int64
		Claimed  int64
		Type     string
		TextID   int64
		Username string
		Claim    string
	}

	query := `SELECT 
				claimId, 
				CAST(strftime('%s', claimed) AS INT), 
				type, 
				textId, 
				(SELECT username FROM users WHERE userId = c.userId), 
				claim 
			FROM claims c WHERE status > '0'`
	sliceFromDB(&claims, query, nil)

	returnJSON(claims, w)
}

func doneclaim(w http.ResponseWriter, r *http.Request) {
	var claim struct {
		ClaimID int64 `json:"claimID"`
	}
	readBody(r, &claim)
	query := `UPDATE claims SET status = '0' WHERE claimId = $1`
	err(insert(query, false, claim.ClaimID))
}

func uploadava(w http.ResponseWriter, r *http.Request) {
	filename := strconv.FormatInt(ctx("user", r).(ctxData).ID, 10)
	path, uploadError := uploadFile(r, "avatar", "/avatars", filename, "image/jpeg", "image/jpg")
	if uploadError != nil {
		w.WriteHeader(400)
		return
	}
	_, writeError := w.Write([]byte(path))
	err(writeError)
}

func uploadimg(w http.ResponseWriter, r *http.Request) {
	filename := strconv.FormatInt(time.Now().Unix(), 10)
	path, uploadError := uploadFile(r, "image", "/images", filename, "image/jpeg", "image/jpg", "image/gif", "image/png")
	if uploadError != nil {
		w.WriteHeader(400)
		return
	}
	_, writeError := w.Write([]byte(path))
	err(writeError)
}
