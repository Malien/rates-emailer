package main

import (
	"bufio"
	"io/fs"
	"os"
)

type EmailAppend struct {
	email    string
	callback chan<- struct {
		bool
		error
	}
}

type EmailDB struct {
	updateChan chan<- EmailAppend
	set        map[string]struct{}
    // Bonus points: insertion order preservation
	cache      []string
	file       fs.File
}

func NewEmailDB(path string) (*EmailDB, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(file)
	emails := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			emails = append(emails, line)
		}
	}
	if scanner.Err() != nil {
		return nil, scanner.Err()
	}

	updateChan := make(chan EmailAppend)
	set := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		set[email] = struct{}{}
	}

	db = &EmailDB{
		updateChan: updateChan,
		set:        set,
		cache:      emails,
		file:       file,
	}

	go func() {
		for update := range updateChan {
			_, exists := db.set[update.email]
			if !exists {
				db.set[update.email] = struct{}{}
				db.cache = append(db.cache, update.email)
			}
			_, err := file.WriteString(update.email + "\n")
			update.callback <- struct {
				bool
				error
			}{exists, err}
		}
	}()

	return db, nil
}

func (db *EmailDB) Append(email string) (exists bool, err error) {
	callback := make(chan struct {
		bool
		error
	})
	db.updateChan <- EmailAppend{
		email:    email,
		callback: callback,
	}
	res := <-callback
	close(callback)
	return res.bool, res.error
}

// Please don't mutate returned slice.
// I could wrap it into some kind of ReadonlySlice, but nah.
func (db *EmailDB) Emails() []string {
	return db.cache
}
