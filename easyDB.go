package main

import (
	"database/sql"
	"errors"
	"os"
	"reflect"

	_ "github.com/mattn/go-sqlite3"
)

// Create db if not exists
func createDB(initialQuery string) {
	fileExists := func(filename string) bool {
		info, err := os.Stat(filename)
		if os.IsNotExist(err) {
			return false
		}
		return !info.IsDir()
	}

	if !fileExists(dbname) {
		file, createError := os.Create(dbname)
		err(createError)
		file.Close()

		// Execute initial query
		execError := execQuery(initialQuery)
		err(execError)
	}
}

// Function for INSERT OR CREATE queries
func execQuery(query string, args ...interface{}) error {
	db, databaseError := sql.Open("sqlite3", dbname)
	err(databaseError)
	defer db.Close()
	_, execError := db.Exec(query, args...)
	return execError
}

func insert(query string, condition bool, args ...interface{}) error {
	db, databaseError := sql.Open("sqlite3", dbname)
	err(databaseError)
	defer db.Close()
	tx, txError := db.Begin()
	err(txError)
	_, execError := tx.Exec(query, args...)
	if execError != nil || condition {
		err(tx.Rollback())
		if execError == nil {
			return errors.New("Validity error")
		}
		return execError
	}
	err(tx.Commit())
	return nil
}

func sliceFromDB(model interface{}, query string, fn func(s string) []interface{}, args ...interface{}) {
	db, databaseError := sql.Open("sqlite3", dbname)
	err(databaseError)
	defer db.Close()
	statement, stmError := db.Prepare(query)
	err(stmError)
	rows, rowsError := statement.Query(args...)
	err(rowsError)
	defer rows.Close()

	container := reflect.Indirect(reflect.ValueOf(model))
	v := container.Type().Elem()
	len := v.NumField()

	tmp := make([]interface{}, len)
	dest := make([]interface{}, len)
	for i := range tmp {
		dest[i] = &tmp[i]
	}

	for rows.Next() {
		scanError := rows.Scan(dest...)
		err(scanError)
		row := reflect.Indirect(reflect.New(v))
		for i, t := range tmp {
			f := row.Field(i)
			if v.Field(i).Type.Kind() == reflect.Slice {
				for _, x := range fn(t.(string)) {
					f.Set(reflect.Append(f, reflect.ValueOf(x)))
				}
			} else {
				f.Set(reflect.ValueOf(t))
			}
		}
		container.Set(reflect.Append(container, row))
	}
}

func isInDB(query string, data interface{}) bool {
	db, databaseError := sql.Open("sqlite3", dbname)
	err(databaseError)
	defer db.Close()

	queryError := db.QueryRow(query, data).Scan(&data)
	if queryError != nil {
		if queryError != sql.ErrNoRows {
			panic(err)
		}
		return false
	}
	return true
}
