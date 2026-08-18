.DEFAULT_GOAL := help

SCALE ?= 1
DATABASE_URL ?= sakuravel:password@tcp(db:3306)/sakuravel?parseTime=true&charset=utf8mb4
TEST_DATABASE_URL ?= sakuravel:password@tcp(localhost:3307)/sakuravel?parseTime=true&charset=utf8mb4

API_URL ?= http://localhost:8080
USERNAME ?= test
DISPLAY_NAME ?= テストユーザー
EMAIL ?= test@example.com
PASSWORD ?= test

.PHONY: help
help: ## 利用可能なコマンド一覧を表示
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: up
up: ## コンテナをバックグラウンド起動 (api, db, frontend)
	docker compose up -d

.PHONY: down
down: ## コンテナを停止・削除 (DBデータは保持)
	docker compose down

.PHONY: restart
restart: ## コンテナを再起動
	docker compose restart

.PHONY: ps
ps: ## コンテナのステータスを確認
	docker compose ps

.PHONY: logs
logs: ## コンテナのログを表示 (例: make logs c=api)
	docker compose logs -f $(c)

.PHONY: build
build: ## コンテナイメージをビルド
	docker compose build

.PHONY: db-reset
db-reset: ## DBボリュームを削除し、初期マイグレーション状態にリセット
	@echo "DBボリュームを削除して初期化します..."
	docker compose down -v
	docker compose up -d db
	@echo "DBの起動を待機中..."
	@until docker compose ps db --format json | grep -q '"Health":"healthy"'; do \
		sleep 1; \
	done
	@echo "DBのリセットが完了しました。"

.PHONY: seed
seed: ## ダミーデータを投入 (例: make seed SCALE=2)
	@echo "ダミーデータを投入中 (scale=$(SCALE))..."
	docker run --rm \
		-v "$$(pwd):/app" \
		-v gomodcache:/go/pkg/mod \
		-w /app \
		-e DATABASE_URL="$(DATABASE_URL)" \
		--network app-network \
		golang:1.25-alpine go run ./seed/main.go -scale $(SCALE)

.PHONY: db-reseed
db-reseed: db-reset seed ## DBリセット後に全サービスを起動し、ダミーデータを投入
	docker compose up -d
	make create-user
	@echo "DBリセット & シードデータ投入 & 全コンテナ起動が完了しました。"

.PHONY: create-user
create-user: ## テスト用ダミーユーザーを作成
	@echo "ユーザーを作成中... (username: $(USERNAME), email: $(EMAIL))"
	@curl -s -w "\nHTTP Status: %{http_code}\n" -X POST "$(API_URL)/register" \
		-H "Content-Type: application/json" \
		-d '{"username":"$(USERNAME)","display_name":"$(DISPLAY_NAME)","email":"$(EMAIL)","password":"$(PASSWORD)"}'

.PHONY: db-cli
db-cli: ## MariaDB クライアントに接続 (sakuravel ユーザー)
	docker compose exec db mariadb -u sakuravel -ppassword sakuravel

.PHONY: db-root
db-root: ## MariaDB クライアントに root ユーザーで接続
	docker compose exec db mariadb -u root -ppassword sakuravel

.PHONY: test-db-up
test-db-up: ## テスト用DBを起動してヘルスチェック待機
	@echo "テスト用DBを起動中 (ポート 3307)..."
	docker compose -f docker-compose.test.yml up -d db-test
	@until docker compose -f docker-compose.test.yml ps db-test --format json | grep -q '"Health":"healthy"'; do \
		sleep 1; \
	done
	@echo "テスト用DBの起動が完了しました。"

.PHONY: test-db-down
test-db-down: ## テスト用DBを停止・ボリューム削除
	docker compose -f docker-compose.test.yml down -v

.PHONY: test
test: test-db-up ## テスト用DBを起動してGoのテストを実行
	DATABASE_URL="$(TEST_DATABASE_URL)" go test -v -race ./...
