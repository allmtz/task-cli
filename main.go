package main

import (
	"os"

	"github.com/boltdb/bolt"
)

func main() {
	// Create a new connection manager to manage the db instance
	mgr := newBoltManager()
	defer mgr.Close()

	// initialize buckets
	mgr.db.Update(func(tx *bolt.Tx) error {
		tx.CreateBucketIfNotExists(TASKS_BUCKET)
		tx.CreateBucketIfNotExists(ARCHIVE_BUCKET)
		return nil
	})

	// create sub commands
	osOut := os.Stdout
	addCmd := newAddCmd(mgr, osOut)
	doCmd := newDoCmd(mgr, osOut)
	updateCmd := newUpdateCmd(mgr, osOut)
	listCmd := newListCmd(mgr, osOut)
	finishCmd := newFinishCmd(mgr, osOut)
	clearCmd := newClearCmd(mgr, osOut)
	archiveCmd := newArchiveCmd(mgr, osOut)
	deleteCmd := newDeleteCmd(mgr, osOut)
	statsCmd := newStatsCmd(mgr, osOut)
	countCmd := newCountCmd(mgr, osOut)
	tagsCmd := newTagsCmd(mgr, osOut)

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
