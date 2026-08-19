package seed

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	appdb "sakuravel/internal/db"
)

const (
	numUsers         = 500
	numPosts         = 10000
	numFollows       = 50000
	numLikes         = 100000
	numReposts       = 20000
	numFootprints    = 20000
	numNotifications = 20000
	batchSize        = 1000

	// 返信が付く投稿の割合
	replyTargetRatio = 0.2
	// 返信にさらに返信が付く割合（これを繰り返すことで深さがランダムになる）
	replyContinueRatio = 0.25
	// 返信をネストさせる深さの上限
	maxReplyDepth = 6
)

var sampleTexts = []string{
	"さくらのクラウドは使いやすいですね！",
	"今日も元気にコーディング中 💻",
	"データベースのインデックスって大事だなと実感",
	"Go言語の並行処理、面白い",
	"新しい機能をリリースしました！",
	"バグを直したと思ったら別のバグが出た",
	"コードレビューありがとうございます",
	"テストを書く習慣をつけたい",
	"インターンシップ楽しい！",
	"クラウドインフラの勉強中",
	"チームランチ最高でした 🍜",
	"OSS活動始めてみようかな",
	"Dockerって便利だなあ",
	"CI/CDパイプライン整備完了",
	"コードの可読性を意識してみる",
	"アーキテクチャ設計って難しい",
	"なんとか本番リリース成功！",
	"ドキュメント整備大事",
	"ペアプロで学びが深まった",
	"エラーログを読む力をつけたい",
}

type SeedResult struct {
	UserIDs         []int64
	PostIDs         []int64
	ReplyIDs        []int64
	FollowCount     int
	LikeCount       int
	RepostCount     int
	FootprintCount  int
	NotifCount      int
	ReplyNotifCount int64
	Duration        time.Duration
}

// InsertSeedData は指定されたスケールでシードデータを投入する
func InsertSeedData(db *sql.DB, scale int) (*SeedResult, error) {
	if scale <= 0 {
		scale = 1
	}

	users := numUsers * scale
	posts := numPosts * scale
	follows := numFollows * scale
	likes := numLikes * scale
	reposts := numReposts * scale
	footprints := numFootprints * scale
	notifications := numNotifications * scale

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("DB接続失敗: %w", err)
	}

	if err := appdb.RunMigrations(); err != nil {
		return nil, fmt.Errorf("マイグレーション失敗: %w", err)
	}

	rng := rand.New(rand.NewSource(42))

	log.Println("=== シードデータ生成開始 ===")
	start := time.Now()

	log.Printf("ユーザー %d 件 作成中...", users)
	userIDs, err := insertUsers(db, rng, users)
	if err != nil {
		return nil, fmt.Errorf("insert users failed: %w", err)
	}

	log.Printf("投稿 %d 件 作成中...", posts)
	postIDs, err := insertPosts(db, rng, userIDs, posts)
	if err != nil {
		return nil, fmt.Errorf("insert posts failed: %w", err)
	}

	log.Printf("返信 作成中（投稿の %.0f%% に返信、深さは最大 %d）...", replyTargetRatio*100, maxReplyDepth)
	replyIDs, err := insertReplies(db, rng, userIDs, postIDs)
	if err != nil {
		return nil, fmt.Errorf("insert replies failed: %w", err)
	}
	log.Printf("  返信 %d 件 作成完了", len(replyIDs))

	log.Printf("フォロー %d 件 作成中...", follows)
	if err := insertFollows(db, rng, userIDs, follows); err != nil {
		return nil, fmt.Errorf("insert follows failed: %w", err)
	}

	log.Printf("いいね %d 件 作成中...", likes)
	if err := insertLikes(db, rng, userIDs, postIDs, likes); err != nil {
		return nil, fmt.Errorf("insert likes failed: %w", err)
	}

	log.Printf("リポスト %d 件 作成中...", reposts)
	if err := insertReposts(db, rng, userIDs, postIDs, reposts); err != nil {
		return nil, fmt.Errorf("insert reposts failed: %w", err)
	}

	log.Printf("足跡 %d 件 作成中...", footprints)
	if err := insertFootprints(db, rng, userIDs, footprints); err != nil {
		return nil, fmt.Errorf("insert footprints failed: %w", err)
	}

	log.Printf("通知 %d 件 作成中...", notifications)
	if err := insertNotifications(db, rng, userIDs, postIDs, notifications); err != nil {
		return nil, fmt.Errorf("insert notifications failed: %w", err)
	}

	log.Println("返信通知 作成中...")
	replyNotifCount, err := insertReplyNotifications(db)
	if err != nil {
		return nil, fmt.Errorf("insert reply notifications failed: %w", err)
	}
	log.Printf("  返信通知 %d 件 作成完了", replyNotifCount)

	duration := time.Since(start)
	log.Printf("=== 完了 (%.1f秒) ===", duration.Seconds())

	return &SeedResult{
		UserIDs:         userIDs,
		PostIDs:         postIDs,
		ReplyIDs:        replyIDs,
		FollowCount:     follows,
		LikeCount:       likes,
		RepostCount:     reposts,
		FootprintCount:  footprints,
		NotifCount:      notifications,
		ReplyNotifCount: replyNotifCount,
		Duration:        duration,
	}, nil
}

func insertUsers(db *sql.DB, rng *rand.Rand, n int) ([]int64, error) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	hashStr := string(hash)

	ids := make([]int64, 0, n)
	for i := 0; i < n; i += batchSize {
		end := i + batchSize
		if end > n {
			end = n
		}

		tx, err := db.Begin()
		if err != nil {
			return nil, fmt.Errorf("users begin: %w", err)
		}

		vals := make([]string, 0, end-i)
		args := make([]any, 0, (end-i)*5)
		for j := i; j < end; j++ {
			username := fmt.Sprintf("user%05d", j+1)
			displayName := fmt.Sprintf("ユーザー%d", j+1)
			bio := fmt.Sprintf("%dのプロフィール文です", j+1)
			email := fmt.Sprintf("user%05d@example.com", j+1)
			vals = append(vals, "(?,?,?,?,?)")
			args = append(args, username, email, hashStr, displayName, bio)
		}

		query := "INSERT INTO users (username, email, password_hash, display_name, bio) VALUES " +
			strings.Join(vals, ",")
		res, err := tx.Exec(query, args...)
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("users insert: %w", err)
		}
		firstID, err := res.LastInsertId()
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("users last insert id: %w", err)
		}
		rowsAffected, _ := res.RowsAffected()
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("users commit: %w", err)
		}

		for k := int64(0); k < rowsAffected; k++ {
			ids = append(ids, firstID+k)
		}

		if (i/batchSize+1)%10 == 0 {
			log.Printf("  users: %d/%d", end, n)
		}
	}
	return ids, nil
}

func insertPosts(db *sql.DB, rng *rand.Rand, userIDs []int64, n int) ([]int64, error) {
	ids := make([]int64, 0, n)
	for i := 0; i < n; i += batchSize {
		end := i + batchSize
		if end > n {
			end = n
		}

		tx, err := db.Begin()
		if err != nil {
			return nil, fmt.Errorf("posts begin: %w", err)
		}

		vals := make([]string, 0, end-i)
		args := make([]any, 0, (end-i)*2)
		for j := i; j < end; j++ {
			uid := userIDs[rng.Intn(len(userIDs))]
			text := sampleTexts[rng.Intn(len(sampleTexts))]
			vals = append(vals, "(?,?)")
			args = append(args, uid, text)
		}

		query := "INSERT INTO posts (user_id, content) VALUES " +
			strings.Join(vals, ",")
		res, err := tx.Exec(query, args...)
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("posts insert: %w", err)
		}
		firstID, err := res.LastInsertId()
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("posts last insert id: %w", err)
		}
		rowsAffected, _ := res.RowsAffected()
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("posts commit: %w", err)
		}

		for k := int64(0); k < rowsAffected; k++ {
			ids = append(ids, firstID+k)
		}

		if (i/batchSize+1)%20 == 0 {
			log.Printf("  posts: %d/%d", end, n)
		}
	}
	return ids, nil
}

func insertReplies(db *sql.DB, rng *rand.Rand, userIDs, postIDs []int64) ([]int64, error) {
	all := make([]int64, 0)

	parents := make([]int64, 0, len(postIDs))
	for _, id := range postIDs {
		if rng.Float64() < replyTargetRatio {
			parents = append(parents, id)
		}
	}

	for depth := 1; depth <= maxReplyDepth && len(parents) > 0; depth++ {
		targets := make([]int64, 0, len(parents)*2)
		for _, id := range parents {
			for k := replyFanout(rng); k > 0; k-- {
				targets = append(targets, id)
			}
		}

		created, err := insertReplyBatch(db, rng, userIDs, targets)
		if err != nil {
			return nil, err
		}
		all = append(all, created...)
		log.Printf("  replies: 深さ%d に %d 件", depth, len(created))

		next := make([]int64, 0, len(created))
		for _, id := range created {
			if rng.Float64() < replyContinueRatio {
				next = append(next, id)
			}
		}
		parents = next
	}
	return all, nil
}

func insertReplyNotifications(db *sql.DB) (int64, error) {
	res, err := db.Exec(`
		INSERT INTO notifications (user_id, type, actor_id, post_id)
		SELECT parent.user_id, 'reply', child.user_id, child.id
		FROM posts child
		JOIN posts parent ON parent.id = child.parent_post_id
		WHERE child.parent_post_id IS NOT NULL
		  AND parent.user_id <> child.user_id
	`)
	if err != nil {
		return 0, fmt.Errorf("reply notifications insert: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func replyFanout(rng *rand.Rand) int {
	switch r := rng.Float64(); {
	case r < 0.55:
		return 1
	case r < 0.85:
		return 2
	case r < 0.96:
		return 3
	default:
		return 4
	}
}

func insertReplyBatch(db *sql.DB, rng *rand.Rand, userIDs, targets []int64) ([]int64, error) {
	ids := make([]int64, 0, len(targets))
	for i := 0; i < len(targets); i += batchSize {
		end := i + batchSize
		if end > len(targets) {
			end = len(targets)
		}

		tx, err := db.Begin()
		if err != nil {
			return nil, fmt.Errorf("replies begin: %w", err)
		}

		vals := make([]string, 0, end-i)
		args := make([]any, 0, (end-i)*3)
		for j := i; j < end; j++ {
			uid := userIDs[rng.Intn(len(userIDs))]
			text := sampleTexts[rng.Intn(len(sampleTexts))]
			vals = append(vals, "(?,?,?)")
			args = append(args, uid, text, targets[j])
		}

		query := "INSERT INTO posts (user_id, content, parent_post_id) VALUES " +
			strings.Join(vals, ",")
		res, err := tx.Exec(query, args...)
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("replies insert: %w", err)
		}
		firstID, err := res.LastInsertId()
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("replies last insert id: %w", err)
		}
		rowsAffected, _ := res.RowsAffected()
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("replies commit: %w", err)
		}

		for k := int64(0); k < rowsAffected; k++ {
			ids = append(ids, firstID+k)
		}
	}
	return ids, nil
}

func insertFollows(db *sql.DB, rng *rand.Rand, userIDs []int64, n int) error {
	seen := make(map[[2]int64]bool, n)
	inserted := 0

	for inserted < n {
		batchEnd := inserted + batchSize
		if batchEnd > n {
			batchEnd = n
		}
		count := batchEnd - inserted

		vals := make([]string, 0, count)
		args := make([]any, 0, count*2)
		added := 0
		for added < count {
			follower := userIDs[rng.Intn(len(userIDs))]
			followee := userIDs[rng.Intn(len(userIDs))]
			if follower == followee {
				continue
			}
			key := [2]int64{follower, followee}
			if seen[key] {
				continue
			}
			seen[key] = true
			vals = append(vals, "(?,?)")
			args = append(args, follower, followee)
			added++
		}

		if len(vals) == 0 {
			break
		}

		query := "INSERT INTO follows (follower_id, followee_id) VALUES " +
			strings.Join(vals, ",") +
			" ON DUPLICATE KEY UPDATE follower_id = follower_id"
		if _, err := db.Exec(query, args...); err != nil {
			return fmt.Errorf("follows insert: %w", err)
		}
		inserted += added
		if inserted%(batchSize*10) == 0 {
			log.Printf("  follows: %d/%d", inserted, n)
		}
	}
	return nil
}

func insertLikes(db *sql.DB, rng *rand.Rand, userIDs, postIDs []int64, n int) error {
	seen := make(map[[2]int64]bool, n)
	inserted := 0

	for inserted < n {
		batchEnd := inserted + batchSize
		if batchEnd > n {
			batchEnd = n
		}
		count := batchEnd - inserted

		vals := make([]string, 0, count)
		args := make([]any, 0, count*2)
		added := 0
		for added < count {
			uid := userIDs[rng.Intn(len(userIDs))]
			pid := postIDs[rng.Intn(len(postIDs))]
			key := [2]int64{uid, pid}
			if seen[key] {
				continue
			}
			seen[key] = true
			vals = append(vals, "(?,?)")
			args = append(args, uid, pid)
			added++
		}

		if len(vals) == 0 {
			break
		}

		query := "INSERT INTO likes (user_id, post_id) VALUES " +
			strings.Join(vals, ",") +
			" ON DUPLICATE KEY UPDATE user_id = user_id"
		if _, err := db.Exec(query, args...); err != nil {
			return fmt.Errorf("likes insert: %w", err)
		}
		inserted += added
		if inserted%(batchSize*20) == 0 {
			log.Printf("  likes: %d/%d", inserted, n)
		}
	}
	return nil
}

func insertReposts(db *sql.DB, rng *rand.Rand, userIDs, postIDs []int64, n int) error {
	seen := make(map[[2]int64]bool, n)
	inserted := 0

	for inserted < n {
		batchEnd := inserted + batchSize
		if batchEnd > n {
			batchEnd = n
		}
		count := batchEnd - inserted

		vals := make([]string, 0, count)
		args := make([]any, 0, count*2)
		added := 0
		for added < count {
			uid := userIDs[rng.Intn(len(userIDs))]
			pid := postIDs[rng.Intn(len(postIDs))]
			key := [2]int64{uid, pid}
			if seen[key] {
				continue
			}
			seen[key] = true
			vals = append(vals, "(?,?)")
			args = append(args, uid, pid)
			added++
		}

		if len(vals) == 0 {
			break
		}

		query := "INSERT INTO reposts (user_id, post_id) VALUES " +
			strings.Join(vals, ",") +
			" ON DUPLICATE KEY UPDATE user_id = user_id"
		if _, err := db.Exec(query, args...); err != nil {
			return fmt.Errorf("reposts insert: %w", err)
		}
		inserted += added
		if inserted%(batchSize*10) == 0 {
			log.Printf("  reposts: %d/%d", inserted, n)
		}
	}
	return nil
}

func pickOther(rng *rand.Rand, userIDs []int64, base int64) int64 {
	if len(userIDs) < 2 {
		return base
	}
	for {
		id := userIDs[rng.Intn(len(userIDs))]
		if id != base {
			return id
		}
	}
}

func insertFootprints(db *sql.DB, rng *rand.Rand, userIDs []int64, n int) error {
	inserted := 0
	for inserted < n {
		end := inserted + batchSize
		if end > n {
			end = n
		}
		count := end - inserted

		vals := make([]string, 0, count)
		args := make([]any, 0, count*2)
		for j := 0; j < count; j++ {
			uid := userIDs[rng.Intn(len(userIDs))]
			vid := pickOther(rng, userIDs, uid)
			vals = append(vals, "(?,?)")
			args = append(args, uid, vid)
		}

		query := "INSERT INTO footprints (user_id, visitor_id) VALUES " + strings.Join(vals, ",")
		if _, err := db.Exec(query, args...); err != nil {
			return fmt.Errorf("footprints insert: %w", err)
		}
		inserted += count
		if inserted%(batchSize*10) == 0 {
			log.Printf("  footprints: %d/%d", inserted, n)
		}
	}
	return nil
}

var notifTypes = []string{"like", "follow", "repost", "footprint"}

func insertNotifications(db *sql.DB, rng *rand.Rand, userIDs, postIDs []int64, n int) error {
	inserted := 0
	for inserted < n {
		end := inserted + batchSize
		if end > n {
			end = n
		}
		count := end - inserted

		vals := make([]string, 0, count)
		args := make([]any, 0, count*4)
		for j := 0; j < count; j++ {
			uid := userIDs[rng.Intn(len(userIDs))]
			actorID := pickOther(rng, userIDs, uid)
			ntype := notifTypes[rng.Intn(len(notifTypes))]
			var postID *int64
			if ntype == "like" || ntype == "repost" {
				pid := postIDs[rng.Intn(len(postIDs))]
				postID = &pid
			}
			vals = append(vals, "(?,?,?,?)")
			args = append(args, uid, ntype, actorID, postID)
		}

		query := "INSERT INTO notifications (user_id, type, actor_id, post_id) VALUES " + strings.Join(vals, ",")
		if _, err := db.Exec(query, args...); err != nil {
			return fmt.Errorf("notifications insert: %w", err)
		}
		inserted += count
		if inserted%(batchSize*10) == 0 {
			log.Printf("  notifications: %d/%d", inserted, n)
		}
	}
	return nil
}
