package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"testing"

	"sakuravel/internal/testutil"
)

func TestAuth_RegisterAndLoginFlow(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ts, _ := testutil.SetupTestServer(t, db)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("failed to create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	// 1. 新規登録 (Register)
	registerPayload := map[string]string{
		"username":     "alice",
		"display_name": "Alice in Wonderland",
		"email":        "alice@example.com",
		"password":     "secret1234",
	}
	reqBody, _ := json.Marshal(registerPayload)
	resp, err := client.Post(ts.URL+"/register", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed to register: %v", err)
	}
	defer testutil.CloseBody(resp)

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 201, got %d. body: %s", resp.StatusCode, string(body))
	}

	// 2. /me にアクセスしてログイン状態を確認 (Register時にCookieがセットされているはず)
	meResp, err := client.Get(ts.URL + "/me")
	if err != nil {
		t.Fatalf("failed to get /me: %v", err)
	}
	defer testutil.CloseBody(meResp)

	if meResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for /me, got %d", meResp.StatusCode)
	}

	var meResult struct {
		User struct {
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
		} `json:"user"`
	}
	if err := json.NewDecoder(meResp.Body).Decode(&meResult); err != nil {
		t.Fatalf("failed to decode /me response: %v", err)
	}
	if meResult.User.Username != "alice" || meResult.User.DisplayName != "Alice in Wonderland" {
		t.Errorf("unexpected user in /me: %+v", meResult.User)
	}

	// 3. ログアウト (Logout)
	logoutResp, err := client.Post(ts.URL+"/logout", "application/json", nil)
	if err != nil {
		t.Fatalf("failed to logout: %v", err)
	}
	defer testutil.CloseBody(logoutResp)

	if logoutResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for logout, got %d", logoutResp.StatusCode)
	}

	// 4. ログアウト後に /me へアクセス -> 401 になるはず
	afterLogoutResp, err := client.Get(ts.URL + "/me")
	if err != nil {
		t.Fatalf("failed to get /me after logout: %v", err)
	}
	defer testutil.CloseBody(afterLogoutResp)

	if afterLogoutResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for /me after logout, got %d", afterLogoutResp.StatusCode)
	}

	// 5. ログイン (Login)
	loginPayload := map[string]string{
		"email":    "alice@example.com",
		"password": "secret1234",
	}
	loginBody, _ := json.Marshal(loginPayload)
	loginResp, err := client.Post(ts.URL+"/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("failed to login: %v", err)
	}
	defer testutil.CloseBody(loginResp)

	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for login, got %d", loginResp.StatusCode)
	}

	// 再度 /me を叩いてログイン成功を確認
	reMeResp, err := client.Get(ts.URL + "/me")
	if err != nil {
		t.Fatalf("failed to get /me after re-login: %v", err)
	}
	defer testutil.CloseBody(reMeResp)
	if reMeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for /me after re-login, got %d", reMeResp.StatusCode)
	}
}

func TestAuth_InvalidScenarios(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ts, _ := testutil.SetupTestServer(t, db)
	client := ts.Client()

	t.Run("重複登録で409が返る", func(t *testing.T) {
		payload := map[string]string{
			"username":     "bob",
			"display_name": "Bob",
			"email":        "bob@example.com",
			"password":     "password123",
		}
		body, _ := json.Marshal(payload)
		resp1, err := client.Post(ts.URL+"/register", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("register 1 failed: %v", err)
		}
		testutil.CloseBody(resp1)
		if resp1.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp1.StatusCode)
		}

		// 同じ username で再登録
		resp2, err := client.Post(ts.URL+"/register", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("register 2 failed: %v", err)
		}
		defer testutil.CloseBody(resp2)
		if resp2.StatusCode != http.StatusConflict {
			t.Errorf("expected 409 Conflict for duplicate registration, got %d", resp2.StatusCode)
		}
	})

	t.Run("パスワード間違いで401が返る", func(t *testing.T) {
		payload := map[string]string{
			"email":    "bob@example.com",
			"password": "wrongpassword",
		}
		body, _ := json.Marshal(payload)
		resp, err := client.Post(ts.URL+"/login", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("login failed: %v", err)
		}
		defer testutil.CloseBody(resp)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("未認証のGET /me で401が返る", func(t *testing.T) {
		resp, err := client.Get(ts.URL + "/me")
		if err != nil {
			t.Fatalf("GET /me failed: %v", err)
		}
		defer testutil.CloseBody(resp)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for unauthenticated /me, got %d", resp.StatusCode)
		}
	})
}
