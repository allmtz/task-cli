package main

import (
	"github.com/boltdb/bolt"
)

func main() {
	// initialize buckets
	db := Connect()
	db.Update(func(tx *bolt.Tx) error {
		tx.CreateBucketIfNotExists(TASKS_BUCKET)
		tx.CreateBucketIfNotExists(ARCHIVE_BUCKET)
		return nil
	})
	db.Close()

	// initialize cobra
	Execute()
}
