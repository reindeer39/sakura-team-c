package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"sakuravel/internal/middleware"
	"sakuravel/internal/model"
	"sakuravel/internal/realtime"
	"strconv"
	"strings"
)

type Handler struct {
	DB *sql.DB
	// CookieSecure が true の場合、セッションCookieに Secure + SameSite=Strict を付与する。
	// フロントエンドとバックエンドが別オリジン（別サブドメイン等）で動く構成向け。
	CookieSecure bool
	// Notifications はユーザーIDごと、Threads はスレッドのルート投稿IDごとの SSE 購読を管理する。
	Notifications *realtime.Hub
	Threads       *realtime.Hub
	Logger        *slog.Logger
}

func (h *Handler) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

func (h *Handler) respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (h *Handler) respondError(w http.ResponseWriter, status int, msg string) {
	if status >= http.StatusInternalServerError {
		h.logger().Error("handler error", "status", status, "error", msg)
	} else if status >= http.StatusBadRequest {
		h.logger().Warn("handler client error", "status", status, "error", msg)
	}
	h.respondJSON(w, status, map[string]string{"error": msg})
}

func (h *Handler) respondErrorWithErr(r *http.Request, w http.ResponseWriter, status int, msg string, err error, attrs ...any) {
	logAttrs := []any{"status", status, "client_message", msg}
	if err != nil {
		logAttrs = append(logAttrs, "error", err.Error())
	}
	if r != nil {
		logAttrs = append(logAttrs, "method", r.Method, "path", r.URL.Path)
		if uid, ok := h.currentUserID(r); ok {
			logAttrs = append(logAttrs, "user_id", uid)
		}
	}
	logAttrs = append(logAttrs, attrs...)

	if status >= http.StatusInternalServerError {
		h.logger().ErrorContext(r.Context(), "handler error", logAttrs...)
	} else {
		h.logger().WarnContext(r.Context(), "handler client error", logAttrs...)
	}
	h.respondJSON(w, status, map[string]string{"error": msg})
}

func (h *Handler) currentUserID(r *http.Request) (int64, bool) {
	id, ok := r.Context().Value(middleware.UserIDKey).(int64)
	return id, ok
}

func (h *Handler) pagination(r *http.Request) (page, perPage, offset int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 50 {
		perPage = 20
	}
	offset = (page - 1) * perPage
	return
}

// fetchUser は users テーブルから1件取得する
func (h *Handler) fetchUser(r *http.Request, userID int64) (model.User, error) {
	var u model.User
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, username, display_name, bio, created_at FROM users WHERE id = ?`,
		userID,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Bio, &u.CreatedAt)
	if err != nil {
		return u, err
	}
	u.AvatarColor = model.AvatarColor(u.ID)

	// フォロワー数・フォロー数・投稿数を取得
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM follows WHERE followee_id = ?`, u.ID,
	).Scan(&u.FollowersCount); err != nil {
		return u, err
	}

	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM follows WHERE follower_id = ?`, u.ID,
	).Scan(&u.FollowingCount); err != nil {
		return u, err
	}

	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM posts WHERE user_id = ?`, u.ID,
	).Scan(&u.PostCount); err != nil {
		return u, err
	}

	if viewerID, ok := h.currentUserID(r); ok && viewerID != u.ID {
		if err := h.DB.QueryRowContext(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM follows WHERE follower_id = ? AND followee_id = ?)`,
			viewerID, u.ID,
		).Scan(&u.FollowedByMe); err != nil {
			return u, err
		}
	}

	return u, nil
}

// fetchUsersInBatch は Usersテーブルから指定されたユーザIDからユーザ情報達を出力する
func (h *Handler) fetchUsersInBatch(r *http.Request, userIDs []int64) (map[int64]model.User, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	users := make(map[int64]model.User)
	placeholders := make([]string, len(userIDs))
	args := make([]any, len(userIDs))

	for i, id := range userIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `
		SELECT id, username, display_name, bio, created_at 
		FROM users
		WHERE id IN (` + strings.Join(placeholders, ",") + `)
		`
	rows, err := h.DB.QueryContext(r.Context(), query, args...)

	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var u model.User

		err := rows.Scan(
			&u.ID,
			&u.Username,
			&u.DisplayName,
			&u.Bio,
			&u.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		u.AvatarColor = model.AvatarColor(u.ID)

		users[u.ID] = u
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := rows.Close(); err != nil {
		return nil, err
	}

	followerCounts, err := h.fetchFollowerCountsInBatch(r, userIDs)
	if err != nil {
		return nil, err
	}

	followingCounts, err := h.fetchFollowingCountsInBatch(r, userIDs)
	if err != nil {
		return nil, err
	}

	postCounts, err := h.fetchPostCountsInBatch(r, userIDs)
	if err != nil {
		return nil, err
	}

	followedByMe, err := h.fetchFollowedByMeInBatch(r, userIDs)
	if err != nil {
		return nil, err
	}

	for userID, u := range users {
		u.FollowersCount = followerCounts[userID]
		u.FollowingCount = followingCounts[userID]
		u.PostCount = postCounts[userID]
		u.FollowedByMe = followedByMe[userID]

		users[userID] = u
	}

	return users, nil
}

// fetchFollowerCountsInBatch は follows テーブルから各ユーザのフォロワー数を全件取得し、ユーザIDと紐づけ
func (h *Handler) fetchFollowerCountsInBatch(r *http.Request, users []int64) (map[int64]int, error) {
	counts := make(map[int64]int)
	if len(users) == 0 {
		return counts, nil
	}
	placeholders := make([]string, len(users))
	args := make([]any, len(users))
	for i, id := range users {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `SELECT followee_id, COUNT(*)
			FROM follows
			WHERE followee_id IN (` + strings.Join(placeholders, ",") + `) 
			GROUP BY followee_id`
	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var userID int64
		var count int

		if err := rows.Scan(&userID, &count); err != nil {
			return nil, err
		}

		counts[userID] = count
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return counts, nil
}

// fetchFollowingCountsInBatch は follows テーブルから各ユーザのフォロー数を全件取得し、ユーザIDと紐づけ
func (h *Handler) fetchFollowingCountsInBatch(r *http.Request, users []int64) (map[int64]int, error) {
	counts := make(map[int64]int)
	if len(users) == 0 {
		return counts, nil
	}
	placeholders := make([]string, len(users))
	args := make([]any, len(users))
	for i, id := range users {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `SELECT follower_id, COUNT(*)
			FROM follows
			WHERE follower_id IN (` + strings.Join(placeholders, ",") + `) 
			GROUP BY follower_id`
	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var userID int64
		var count int

		if err := rows.Scan(&userID, &count); err != nil {
			return nil, err
		}

		counts[userID] = count
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return counts, nil
}

// fetchPostCountsInBatch は posts テーブルから各ユーザの投稿数を全件取得し、ユーザIDと紐づけ
func (h *Handler) fetchPostCountsInBatch(r *http.Request, users []int64) (map[int64]int, error) {
	counts := make(map[int64]int)
	if len(users) == 0 {
		return counts, nil
	}
	placeholders := make([]string, len(users))
	args := make([]any, len(users))
	for i, id := range users {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `SELECT user_id, COUNT(*)
			FROM posts
			WHERE user_id IN (` + strings.Join(placeholders, ",") + `) 
			GROUP BY user_id`
	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var userID int64
		var count int

		if err := rows.Scan(&userID, &count); err != nil {
			return nil, err
		}

		counts[userID] = count
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return counts, nil
}

// fetchFollowedByMeInBatch は、現在のユーザが各ユーザをフォローしているか一括取得する
func (h *Handler) fetchFollowedByMeInBatch(r *http.Request, users []int64) (map[int64]bool, error) {
	followed := make(map[int64]bool)
	if len(users) == 0 {
		return followed, nil
	}
	viewerID, ok := h.currentUserID(r)
	if !ok {
		return followed, nil
	}
	placeholders := make([]string, len(users))
	args := make([]any, 0, len(users)+1)

	args = append(args, viewerID)

	for i, id := range users {
		placeholders[i] = "?"
		args = append(args, id)

		// 最初は全員false
		followed[id] = false
	}

	query := `
        SELECT followee_id
        FROM follows
        WHERE follower_id = ?
          AND followee_id IN (` + strings.Join(placeholders, ",") + `)
    `

	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var userID int64

		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}

		followed[userID] = true
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return followed, nil
}

// fetchPost は posts テーブルから1件取得し、関連データを付加する
func (h *Handler) fetchPost(r *http.Request, postID, viewerID int64) (model.Post, error) {
	var p model.Post
	var userID int64
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, user_id, content, is_repost, original_post_id, parent_post_id, created_at
		 FROM posts WHERE id = ?`,
		postID,
	).Scan(&p.ID, &userID, &p.Content, &p.IsRepost, &p.OriginalPostID, &p.ParentPostID, &p.CreatedAt)
	if err != nil {
		return p, err
	}

	author, err := h.fetchUser(r, userID)
	if err != nil {
		return p, err
	}
	p.Author = author

	h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM likes WHERE post_id = ?`, p.ID,
	).Scan(&p.LikesCount)

	h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM reposts WHERE post_id = ?`, p.ID,
	).Scan(&p.RepostsCount)

	p.RepliesCount = h.countReplies(r, p.ID, 0)

	if viewerID > 0 {
		if err := h.DB.QueryRowContext(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM likes WHERE user_id = ? AND post_id = ?)`,
			viewerID, p.ID,
		).Scan(&p.LikedByMe); err != nil {
			return p, err
		}

		if err := h.DB.QueryRowContext(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM reposts WHERE user_id = ? AND post_id = ?)`,
			viewerID, p.ID,
		).Scan(&p.RepostedByMe); err != nil {
			return p, err
		}
	}

	// 返信の場合、返信先の投稿者を解決する
	if p.ParentPostID != nil {
		var username, displayName string
		err := h.DB.QueryRowContext(r.Context(), `
			SELECT u.username, u.display_name
			FROM posts parent JOIN users u ON u.id = parent.user_id
			WHERE parent.id = ?
		`, *p.ParentPostID).Scan(&username, &displayName)
		if err == nil {
			p.ReplyToUsername = &username
			p.ReplyToDisplayName = &displayName
		}
	}

	// リポストの場合、何をリポストしたか分かるように元投稿を解決する
	if p.IsRepost && p.OriginalPostID != nil && *p.OriginalPostID != p.ID {
		if original, err := h.fetchPost(r, *p.OriginalPostID, viewerID); err == nil {
			p.OriginalPost = &original
		}
	}

	return p, nil
}

// fetchPostsInBatch は posts テーブルから指定された範囲取得し、関連データを付加する
func (h *Handler) fetchPostsInBatch(r *http.Request, ids []int64, viewerID int64) ([]model.Post, error) {
	if len(ids) == 0 {
		return []model.Post{}, nil
	}
	var posts []model.Post

	postUserIDs := make(map[int64]int64)
	var userIDs []int64
	var parentPostIDs []int64
	var originalPostIDs []int64

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))

	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `
		SELECT id, user_id, content, is_repost, original_post_id, parent_post_id, created_at
		FROM posts
		WHERE id IN (` + strings.Join(placeholders, ",") + `)
		`
	rows, err := h.DB.QueryContext(r.Context(), query, args...)

	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var p model.Post
		var userID int64

		if err := rows.Scan(
			&p.ID,
			&userID,
			&p.Content,
			&p.IsRepost,
			&p.OriginalPostID,
			&p.ParentPostID,
			&p.CreatedAt,
		); err != nil {
			return nil, err
		}
		if p.ParentPostID != nil {
			parentPostIDs = append(parentPostIDs, *p.ParentPostID)
		}

		if p.IsRepost && p.OriginalPostID != nil && *p.OriginalPostID != p.ID {
			originalPostIDs = append(originalPostIDs, *p.OriginalPostID)
		}

		postUserIDs[p.ID] = userID
		userIDs = append(userIDs, userID)

		posts = append(posts, p)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := rows.Close(); err != nil {
		return nil, err
	}
	originalPosts, err := h.fetchPostsInBatch(r, originalPostIDs, viewerID)
	if err != nil {
		return nil, err
	}
	originalPostByID := make(map[int64]model.Post, len(originalPosts))
	for _, original := range originalPosts {
		originalPostByID[original.ID] = original
	}

	replyTargets, err := h.fetchReplyTargetsInBatch(r, parentPostIDs)
	if err != nil {
		return nil, err
	}

	authors, err := h.fetchUsersInBatch(r, userIDs)
	if err != nil {
		return nil, err
	}

	likeCounts, err := h.fetchLikeCountsInBatch(r, ids)
	if err != nil {
		return nil, err
	}

	repostCounts, err := h.fetchRepostsCountsInBatch(r, ids)
	if err != nil {
		return nil, err
	}

	var likedByMe map[int64]bool
	var repostedByMe map[int64]bool
	if viewerID > 0 {
		likedByMe, err = h.fetchLikedByMeInBatch(r, ids)
		if err != nil {
			return nil, err
		}

		repostedByMe, err = h.fetchRepostedByMeInBatch(r, ids)
		if err != nil {
			return nil, err
		}
	}

	for i := range posts {
		p := &posts[i]

		userID := postUserIDs[p.ID]

		p.Author = authors[userID]
		p.LikesCount = likeCounts[p.ID]
		p.RepostsCount = repostCounts[p.ID]

		p.LikedByMe = likedByMe[p.ID]
		p.RepostedByMe = repostedByMe[p.ID]
		if p.ParentPostID != nil {
			if target, ok := replyTargets[*p.ParentPostID]; ok {
				username := target.Username
				displayName := target.DisplayName

				p.ReplyToUsername = &username
				p.ReplyToDisplayName = &displayName
			}
		}
		if p.IsRepost &&
			p.OriginalPostID != nil &&
			*p.OriginalPostID != p.ID {

			if original, ok := originalPostByID[*p.OriginalPostID]; ok {
				p.OriginalPost = &original
			}
		}
		p.RepliesCount = h.countReplies(r, p.ID, 0)
	}

	postByID := make(map[int64]model.Post, len(posts))

	for _, p := range posts {
		postByID[p.ID] = p
	}

	orderedPosts := make([]model.Post, 0, len(ids))
	for _, id := range ids {
		if p, ok := postByID[id]; ok {
			orderedPosts = append(orderedPosts, p)
		}
	}

	return orderedPosts, nil
}

// fetchLikeCountsInbatch は posts テーブルから任意件取得し、関連データを付加する
func (h *Handler) fetchLikeCountsInBatch(r *http.Request, postIDs []int64) (map[int64]int, error) {
	counts := make(map[int64]int)
	if len(postIDs) == 0 {
		return counts, nil
	}
	placeholders := make([]string, len(postIDs))
	args := make([]any, len(postIDs))
	for i, id := range postIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `SELECT post_id, COUNT(*)
			FROM likes
			WHERE post_id IN (` + strings.Join(placeholders, ",") + `) 
			GROUP BY post_id`
	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var postID int64
		var count int

		if err := rows.Scan(&postID, &count); err != nil {
			return nil, err
		}

		counts[postID] = count
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return counts, nil
}

// fetchRepostsCountsInbatch は posts テーブルから任意件取得し、関連データを付加する
func (h *Handler) fetchRepostsCountsInBatch(r *http.Request, postIDs []int64) (map[int64]int, error) {
	counts := make(map[int64]int)
	if len(postIDs) == 0 {
		return counts, nil
	}
	placeholders := make([]string, len(postIDs))
	args := make([]any, len(postIDs))
	for i, id := range postIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `SELECT post_id, COUNT(*)
			FROM reposts
			WHERE post_id IN (` + strings.Join(placeholders, ",") + `) 
			GROUP BY post_id`
	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var posts int64
		var count int

		if err := rows.Scan(&posts, &count); err != nil {
			return nil, err
		}

		counts[posts] = count
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return counts, nil
}

// USERがこのポスト達をすでに良いねしているか
func (h *Handler) fetchLikedByMeInBatch(r *http.Request, postIDs []int64) (map[int64]bool, error) {
	liked := make(map[int64]bool)
	if len(postIDs) == 0 {
		return liked, nil
	}
	viewerID, ok := h.currentUserID(r)
	if !ok {
		return liked, nil
	}
	placeholders := make([]string, len(postIDs))
	args := make([]any, 0, len(postIDs)+1)

	args = append(args, viewerID)

	for i, id := range postIDs {
		placeholders[i] = "?"
		args = append(args, id)

		// 最初は全員false
		liked[id] = false
	}

	query := `
        SELECT post_id
        FROM likes
        WHERE user_id = ?
          AND post_id IN (` + strings.Join(placeholders, ",") + `)
    `

	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var userID int64

		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}

		liked[userID] = true
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return liked, nil
}

// USERがこのポスト達をすでにリポストしているか
func (h *Handler) fetchRepostedByMeInBatch(r *http.Request, postIDs []int64) (map[int64]bool, error) {
	reposted := make(map[int64]bool)
	if len(postIDs) == 0 {
		return reposted, nil
	}
	viewerID, ok := h.currentUserID(r)
	if !ok {
		return reposted, nil
	}
	placeholders := make([]string, len(postIDs))
	args := make([]any, 0, len(postIDs)+1)

	args = append(args, viewerID)

	for i, id := range postIDs {
		placeholders[i] = "?"
		args = append(args, id)

		// 最初は全員false
		reposted[id] = false
	}

	query := `
        SELECT post_id
        FROM reposts
        WHERE user_id = ?
          AND post_id IN (` + strings.Join(placeholders, ",") + `)
    `

	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var PostID int64

		if err := rows.Scan(&PostID); err != nil {
			return nil, err
		}

		reposted[PostID] = true
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return reposted, nil
}

// 返信の場合、返信先の投稿者を解決する
type replyTarget struct {
	Username    string
	DisplayName string
}

func (h *Handler) fetchReplyTargetsInBatch(r *http.Request, parentPostIDs []int64) (map[int64]replyTarget, error) {
	targets := make(map[int64]replyTarget)

	if len(parentPostIDs) == 0 {
		return targets, nil
	}

	placeholders := make([]string, len(parentPostIDs))
	args := make([]any, len(parentPostIDs))

	for i, id := range parentPostIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `
        SELECT parent.id, u.username, u.display_name
        FROM posts parent
        JOIN users u ON u.id = parent.user_id
        WHERE parent.id IN (` + strings.Join(placeholders, ",") + `)
    `

	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var parentPostID int64
		var target replyTarget

		if err := rows.Scan(
			&parentPostID,
			&target.Username,
			&target.DisplayName,
		); err != nil {
			return nil, err
		}

		targets[parentPostID] = target
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return targets, nil
}

// maxThreadDepth はスレッドを辿る深さの上限（循環や極端に深いスレッドの保険）。
const maxThreadDepth = 50

// countReplies は投稿にぶら下がる返信の数を返す。ネストした返信も含めた合計。
func (h *Handler) countReplies(r *http.Request, postID int64, depth int) int {
	if depth >= maxThreadDepth {
		return 0
	}

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id FROM posts WHERE parent_post_id = ?`, postID)
	if err != nil {
		return 0
	}

	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()

	total := len(ids)
	for _, id := range ids {
		total += h.countReplies(r, id, depth+1)
	}
	return total
}

// threadRootID はスレッドの起点となる投稿IDを返す。
func (h *Handler) threadRootID(r *http.Request, postID int64) int64 {
	current := postID
	for i := 0; i < maxThreadDepth; i++ {
		var parent *int64
		err := h.DB.QueryRowContext(r.Context(),
			`SELECT parent_post_id FROM posts WHERE id = ?`, current,
		).Scan(&parent)
		if err != nil || parent == nil {
			return current
		}
		current = *parent
	}
	return current
}

func pathID(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(r.PathValue(key), 10, 64)
}
