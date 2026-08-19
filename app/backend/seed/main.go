package main

import (
	"database/sql"
	"flag"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"

	"sakuravel/internal/seed"
)

func main() {
	scale := flag.Int("scale", 1, "データ件数の倍率")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatalf("DATABASE_URL is not set")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if _, err := seed.InsertSeedData(db, *scale); err != nil {
		log.Fatalf("シードデータ投入エラー: %v", err)
	}
}
