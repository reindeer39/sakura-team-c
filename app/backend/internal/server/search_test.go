package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"

	"sakuravel/internal/testutil"
)

func TestSearch_LikeEscape_Posts(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ts, _ := testutil.SetupTestServer(t, db)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	loginHelper(t, ts.URL, client, "search_user", "search_user@example.com", "pass1234")

	// テスト用投稿を作成
	posts := []string{
		"100% genuine product",
		"1000 items in stock",
		"special_offer today",
		"special offer today",
		`path\to\target file`,
		"normal post without special chars",
	}

	for _, content := range posts {
		payload, _ := json.Marshal(map[string]string{"content": content})
		resp, err := client.Post(ts.URL+"/posts", "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("failed to create post: %v", err)
		}
		defer testutil.CloseBody(resp)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
		}
	}

	type searchResult struct {
		Posts []struct {
			ID      int64  `json:"id"`
			Content string `json:"content"`
		} `json:"posts"`
		Total   int `json:"total"`
		Page    int `json:"page"`
		PerPage int `json:"per_page"`
	}

	tests := []struct {
		name          string
		query         string
		expectedTotal int
		expectedMatch string
	}{
		{
			name:          "search with percent (%) - only literal % matches",
			query:         "100%",
			expectedTotal: 1,
			expectedMatch: "100% genuine product",
		},
		{
			name:          "search with underscore (_) - only literal _ matches",
			query:         "special_offer",
			expectedTotal: 1,
			expectedMatch: "special_offer today",
		},
		{
			name:          "search with backslash (\\) - only literal \\ matches",
			query:         `path\to`,
			expectedTotal: 1,
			expectedMatch: `path\to\target file`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchURL := fmt.Sprintf("%s/search?type=posts&q=%s", ts.URL, url.QueryEscape(tt.query))
			resp, err := client.Get(searchURL)
			if err != nil {
				t.Fatalf("failed to search posts: %v", err)
			}
			defer testutil.CloseBody(resp)

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
			}

			var res searchResult
			if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if res.Total != tt.expectedTotal {
				t.Errorf("expected total=%d, got total=%d", tt.expectedTotal, res.Total)
			}
			if len(res.Posts) != tt.expectedTotal {
				t.Fatalf("expected %d posts returned, got %d", tt.expectedTotal, len(res.Posts))
			}
			if tt.expectedTotal > 0 && res.Posts[0].Content != tt.expectedMatch {
				t.Errorf("expected post content %q, got %q", tt.expectedMatch, res.Posts[0].Content)
			}
		})
	}
}

func TestSearch_LikeEscape_Users(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ts, _ := testutil.SetupTestServer(t, db)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// ユーザー作成
	users := []struct {
		username    string
		displayName string
		email       string
	}{
		{"user_alpha", "User Alpha", "alpha@example.com"},
		{"user1alpha", "User 1 Alpha", "alpha1@example.com"},
		{"user_100%", "User 100%", "percent@example.com"},
		{"user_1000", "User 1000", "thousand@example.com"},
	}

	for _, u := range users {
		payload, _ := json.Marshal(map[string]string{
			"username":     u.username,
			"display_name": u.displayName,
			"email":        u.email,
			"password":     "pass1234",
		})
		resp, err := client.Post(ts.URL+"/register", "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("failed to register: %v", err)
		}
		defer testutil.CloseBody(resp)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
		}
	}

	type userSearchResult struct {
		Users []struct {
			ID          int64  `json:"id"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
		} `json:"users"`
		Total int `json:"total"`
	}

	tests := []struct {
		name          string
		query         string
		expectedTotal int
		expectedUser  string
	}{
		{
			name:          "search username with underscore - should not match arbitrary character",
			query:         "user_alpha",
			expectedTotal: 1,
			expectedUser:  "user_alpha",
		},
		{
			name:          "search with percent - should only match literal percent",
			query:         "100%",
			expectedTotal: 1,
			expectedUser:  "user_100%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchURL := fmt.Sprintf("%s/search?type=users&q=%s", ts.URL, url.QueryEscape(tt.query))
			resp, err := client.Get(searchURL)
			if err != nil {
				t.Fatalf("failed to search users: %v", err)
			}
			defer testutil.CloseBody(resp)

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
			}

			var res userSearchResult
			if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if res.Total != tt.expectedTotal {
				t.Errorf("expected total=%d, got total=%d", tt.expectedTotal, res.Total)
			}
			if len(res.Users) != tt.expectedTotal {
				t.Fatalf("expected %d users returned, got %d", tt.expectedTotal, len(res.Users))
			}
			if tt.expectedTotal > 0 && res.Users[0].Username != tt.expectedUser {
				t.Errorf("expected username %q, got %q", tt.expectedUser, res.Users[0].Username)
			}
		})
	}
}
