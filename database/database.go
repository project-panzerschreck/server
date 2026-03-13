package database

import (
	"database/sql"
	_ "embed"
	"sync"

	_ "modernc.org/sqlite"
)

var db *sql.DB
var initializer sync.Once

//go:embed schema.sql
var initializeSql string

func GetDB() *sql.DB {
	initializer.Do(func() {
		var err error
		db, err = sql.Open("sqlite", ":memory:?_texttotime=true")
		if err != nil {
			panic(err)
		}

		_, err = db.Exec(initializeSql)

		if err != nil {
			panic(err)
		}
	})

	return db
}
