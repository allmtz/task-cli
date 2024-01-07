package main

import (
	"os"

	"github.com/boltdb/bolt"
)

func main() {
	// initialize buckets
	db := Connect()
	defer db.Close()
	db.Update(func(tx *bolt.Tx) error {
		tx.CreateBucketIfNotExists(TASKS_BUCKET)
		tx.CreateBucketIfNotExists(ARCHIVE_BUCKET)
		return nil
	})
	// db.Close()

	addCmd := newAddCmd(db, os.Stdout)

	rootCmd.AddCommand(addCmd)

	// initialize cobra
	Execute()
}
