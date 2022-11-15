package main

import (
	"encoding/json"
	"errors"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"unicode"

	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
)

func err(e error) {
	if e != nil {
		panic(e)
	}
}

func readBody(r *http.Request, data interface{}) {
	body, readError := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	err(readError)
	unmarshalError := json.Unmarshal(body, &data)
	err(unmarshalError)
}

type report []string

func (rep *report) regcheck(col, str, regex string) {
	if !regexp.MustCompile(regex).MatchString(str) {
		*rep = append(*rep, col)
	}
}

func returnJSON(d interface{}, w http.ResponseWriter) {
	js, jsonError := json.Marshal(d)
	err(jsonError)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, writeError := w.Write(js)
	err(writeError)
}

func ctx(key ctxKey, r *http.Request) interface{} {
	if v := r.Context().Value(key); v != nil {
		return v
	}
	return nil
}

func addCookie(w http.ResponseWriter, name, value string, exp time.Time) {
	cookie := http.Cookie{
		Name:     name,
		Value:    value,
		Expires:  exp,
		HttpOnly: true,
		Path:     "/",
		// Secure:   true,
	}
	http.SetCookie(w, &cookie)
}

func processCategories(cats []int64) (string, error) {
	var strcats string
	for _, cat := range cats {
		check := "SELECT categoryId FROM categories WHERE categoryId = ?"
		inDB := isInDB(check, cat)
		if !inDB {
			return "", errors.New("No such category!")
		}
		strcats += "\"" + strconv.FormatInt(cat, 10) + "\""
	}
	return strcats, nil
}

func reqQuery(name string, r *http.Request) string {
	switch len(r.FormValue(name)) {
	case 0:
		return "%"
	default:
		return r.FormValue(name)
	}
}

func getCats(c string) []interface{} {

	// Divide string to []string
	cats := strings.FieldsFunc(c, func(c rune) bool {
		return !unicode.IsNumber(c)
	})

	// Get all categories from DB
	var categ []struct {
		ID   int64
		Name string
	}
	catQuery := `SELECT categoryId, name FROM categories`
	sliceFromDB(&categ, catQuery, nil)

	var res []interface{}
	for _, c := range cats {
		for _, k := range categ {
			id, _ := strconv.ParseInt(c, 10, 64)
			if k.ID == id {
				res = append(res, k)
				break
			}
		}
	}
	return res
}

func uploadFile(r *http.Request, formname, path, filename string, formats ...string) (string, error) {
	file, fileHeader, formError := r.FormFile(formname)
	if formError != nil {
		return "", errors.New("invalid file upload")
	}
	defer file.Close()

	// validate file size
	fileSize := fileHeader.Size
	if fileSize > avatarSize {
		return "", errors.New("invalid file size")
	}

	// validate file content
	fileBytes, readError := ioutil.ReadAll(file)
	if readError != nil {
		return "", errors.New("invalid file content")
	}

	// validate file format
	detectedFileType := http.DetectContentType(fileBytes)
	valid := false
	for _, x := range formats {
		if detectedFileType == x {
			valid = true
			break
		}
	}
	if !valid {
		return "", errors.New("wrong file format")
	}

	fileEndings, filetypeError := mime.ExtensionsByType(detectedFileType)
	err(filetypeError)

	format := fileEndings[0]
	if format == ".jpeg" || format == ".jpe" {
		format = ".jpg"
	}

	newPath := filepath.Join("./front"+path, filename+format)

	// write file
	newFile, createError := os.Create(newPath)
	err(createError)
	defer newFile.Close()
	_, writeError := newFile.Write(fileBytes)

	if writeError != nil || newFile.Close() != nil {
		err(writeError)
	}
	return path + "/" + filename + format, nil
}

func regcheck(s, regex string) bool {
	return !regexp.MustCompile(regex).MatchString(s)
}
