package testing

import (
	"database/sql"

	_ "github.com/lib/pq"
)

var tablesToTruncate = []string{
	"playlists",
	"playlists_songs",
	"songs",
}

// DB references a testing database that can be used in the tests for any
// modules that need a database connection.
var DB *sql.DB

func init() {
	var err error
	DB, err = sql.Open("postgres",
		"postgres://localhost/deathguild-test?sslmode=disable")
	if err != nil {
		panic(err)
	}

	TruncateTestDB()
}

// TruncateTestDB truncates all tables in the testing database.
func TruncateTestDB() {
	for _, table := range tablesToTruncate {
		_, err := DB.Exec(`TRUNCATE TABLE ` + table + ` CASCADE`)
		if err != nil {
			panic(err)
		}
	}
}
