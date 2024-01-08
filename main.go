package main

import (
	"os"

	"github.com/boltdb/bolt"
)

func main() {
	db := Connect()
	defer db.Close()

	// initialize buckets
	db.Update(func(tx *bolt.Tx) error {
		tx.CreateBucketIfNotExists(TASKS_BUCKET)
		tx.CreateBucketIfNotExists(ARCHIVE_BUCKET)
		return nil
	})

	// create sub commands
	osOut := os.Stdout
	addCmd := newAddCmd(db, osOut)
	doCmd := newDoCmd(db, osOut)
	updateCmd := newUpdateCmd(db, osOut)
	listCmd := newListCmd(db, osOut)
	finishCmd := newFinishCmd(db, osOut)
	clearCmd := newClearCmd(db, osOut)
	archiveCmd := newArchiveCmd(db, osOut)
	deleteCmd := newDeleteCmd(db, osOut)
	statsCmd := newStatsCmd(db, osOut)
	countCmd := newCountCmd(db, osOut)
	tagsCmd := newTagsCmd(db, osOut)

	// add sub commands
	rootCmd.AddCommand(
		addCmd, doCmd,
		updateCmd, listCmd,
		finishCmd, clearCmd,
		archiveCmd, deleteCmd,
		countCmd, tagsCmd,
		statsCmd,
	)

	// initialize cobra
	Execute()
}
