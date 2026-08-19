package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"testing"

	"sakuravel/internal/testutil"
)

func loginHelper(t *testing.T, baseURL string, client *http.Client, username, email, password string) {
	t.Helper()
	registerPayload := map[string]string{
		"username":     username,
		"display_name": username + "_disp",
		"email":        email,
		"password":     password,
	}
	body, _ := json.Marshal(registerPayload)
	resp, err := client.Post(baseURL+"/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}
	defer testutil.CloseBody(resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created from register, got %d", resp.StatusCode)
	}
}

func TestPost_CreateAndTimeline(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ts, _ := testutil.SetupTestServer(t, db)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	loginHelper(t, ts.URL, client, "carol", "carol@example.com", "pass1234")

	// 1. 投稿作成
	postPayload := map[string]string{
		"content": "Hello World! This is my first test post.",
	}
	reqBody, _ := json.Marshal(postPayload)
	createResp, err := client.Post(ts.URL+"/posts", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed to create post: %v", err)
	}
	defer testutil.CloseBody(createResp)

	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", createResp.StatusCode)
	}

	var createdResult struct {
		Post struct {
			ID      int64  `json:"id"`
			Content string `json:"content"`
		} `json:"post"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createdResult); err != nil {
		t.Fatalf("failed to decode create post response: %v", err)
	}
	if createdResult.Post.Content != "Hello World! This is my first test post." {
		t.Errorf("unexpected post content: %s", createdResult.Post.Content)
	}

	// 2. タイムライン取得 (latest)
	timelineResp, err := client.Get(ts.URL + "/posts?feed=latest")
	if err != nil {
		t.Fatalf("failed to get timeline: %v", err)
	}
	defer testutil.CloseBody(timelineResp)

	if timelineResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from timeline, got %d", timelineResp.StatusCode)
	}

	var timelineResult struct {
		Posts []map[string]any `json:"posts"`
		Total int              `json:"total"`
	}
	if err := json.NewDecoder(timelineResp.Body).Decode(&timelineResult); err != nil {
		t.Fatalf("failed to decode timeline response: %v", err)
	}

	if timelineResult.Total != 1 || len(timelineResult.Posts) != 1 {
		t.Fatalf("expected 1 post in timeline, got total=%d, len=%d", timelineResult.Total, len(timelineResult.Posts))
	}
}

func TestPost_LikeFlow(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ts, _ := testutil.SetupTestServer(t, db)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	loginHelper(t, ts.URL, client, "dave", "dave@example.com", "pass1234")

	// 1. 投稿作成
	postPayload := map[string]string{"content": "Post to be liked"}
	reqBody, _ := json.Marshal(postPayload)
	createResp, err := client.Post(ts.URL+"/posts", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed to create post: %v", err)
	}
	defer testutil.CloseBody(createResp)

	var createdResult struct {
		Post struct {
			ID int64 `json:"id"`
		} `json:"post"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&createdResult)
	postID := createdResult.Post.ID

	// 2. いいね (POST /likes)
	likePayload := map[string]int64{"post_id": postID}
	likeBody, _ := json.Marshal(likePayload)
	likeResp, err := client.Post(ts.URL+"/likes", "application/json", bytes.NewReader(likeBody))
	if err != nil {
		t.Fatalf("failed to like post: %v", err)
	}
	defer testutil.CloseBody(likeResp)
	if likeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from like, got %d", likeResp.StatusCode)
	}

	// 3. いいね解除 (DELETE /likes/{post_id})
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/likes/%d", ts.URL, postID), nil)
	if err != nil {
		t.Fatalf("failed to create delete like request: %v", err)
	}
	unlikeResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to unlike post: %v", err)
	}
	defer testutil.CloseBody(unlikeResp)
	if unlikeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from unlike, got %d", unlikeResp.StatusCode)
	}
}
