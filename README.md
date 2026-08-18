# さくらインターネット　28年度エンジニアインターン

階層は下記の通りになってます：

- /app: アプリケーション
- /infra: インフラ周り（Terraformなど）

## ローカル開発環境のセットアップ

リポジトリ直下の `Makefile` から、CIと同等のチェックをローカルで実行できます。
Goのバージョンは `app/backend/go.mod` に記載されたバージョンを使用してください。
`gofmt` はGo本体に同梱されているため、個別のインストールは不要です。

### macOS

1. [Go公式のダウンロードページ](https://go.dev/dl/)から、`go.mod` に記載されたバージョンのmacOS用パッケージをインストールします。
2. Homebrewで `golangci-lint` をインストールします。

```shell
brew install golangci-lint
```

### Ubuntu（WSL）

次の例では、`go.mod` に記載されたGo 1.25.11をx86-64環境へインストールします。ARM64環境では、ダウンロードするファイル名の `linux-amd64` を `linux-arm64` に置き換えてください。

```shell
sudo apt-get update
sudo apt-get install -y curl make git

curl -LO https://go.dev/dl/go1.25.11.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.25.11.linux-amd64.tar.gz

echo 'export PATH=/usr/local/go/bin:$HOME/go/bin:$PATH' >> ~/.profile
source ~/.profile
```

続けて、CIと同じv2系の `golangci-lint` を公式インストーラーで導入します。

```shell
curl -sSfL https://golangci-lint.run/install.sh \
  | sh -s -- -b "$(go env GOPATH)/bin" v2.12.2
```

### インストール確認

```shell
go version
gofmt -h
golangci-lint version
make --version
```

### Makefileの実行

Go関連のチェックは、リポジトリ直下で次のように実行します。

```shell
make fmt-check
make lint
make test
make build
```

`make fmt` はGoとTerraformのファイルを実際に書き換えるため、実行後に差分を確認してください。

すべてのCI相当チェックをまとめて実行する場合は、DockerとTerraformもインストールしたうえで次を実行します。

```shell
make check
```

`make lint` はローカルの `main` ブランチとの差分を利用するため、事前に `main` ブランチを最新の状態へ更新してください。
