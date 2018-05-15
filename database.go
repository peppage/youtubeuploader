package main

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	*sql.DB
}

type Video struct {
	Filename string
}

func openDatabase(filename string) *Store {
	db, err := sql.Open("sqlite3", filename)
	handleErr(err)
	return &Store{
		db,
	}
}

func (db *Store) initializeDatabase() {
	sqlStmt := `
	create table if not exists
	videos (filename text not null, uploaded bool default false);
	`

	_, err := db.Exec(sqlStmt)
	handleErr(err)
}

func (db *Store) saveVideo(vid Video) {
	stmt, err := db.Prepare("insert into videos(filename) values(?);")
	handleErr(err)

	_, err = stmt.Exec(vid.Filename)
	handleErr(err)
}

func (db *Store) setVideoUploaded(vid Video) {
	stmt, err := db.Prepare("update videos set uploaded = 1 where filename = ?")
	handleErr(err)

	_, err = stmt.Exec(vid.Filename)
	handleErr(err)
}

func (db *Store) getNotUploadedVideos() []*Video {
	stmt, err := db.Prepare("select filename from videos where not uploaded")
	handleErr(err)

	vids := []*Video{}

	rows, err := stmt.Query()
	handleErr(err)
	defer rows.Close()

	for rows.Next() {
		var filename string

		err := rows.Scan(&filename)
		handleErr(err)

		var v = Video{
			Filename: filename,
		}
		vids = append(vids, &v)
	}

	handleErr(err)

	return vids
}
