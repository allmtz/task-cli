package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/boltdb/bolt"
)

func TestUpdateCmd(t *testing.T) {
	db, path := setup()
	defer teardown(db, path)

	// buf will hold any outputs to stdout from the update command, outputs to stderr, and the command "Usage" text,
	// this eliminates the noise when running "$ go test"
	buf := new(bytes.Buffer)
	uCmd := newUpdateCmd(db, buf)
	uCmd.SetOut(buf)
	uCmd.SetErr(buf)

	uCmd.SetArgs([]string{})
	err := uCmd.Execute()
	if err == nil {
		t.Fatalf("Should error when no arguments are passed")
	}

	uCmd.SetArgs([]string{"1", "2"})
	err = uCmd.Execute()
	if err == nil {
		t.Fatalf("Should error when more than 1 argument is passed")
	}

	uCmd.SetArgs([]string{"a"})
	err = uCmd.Execute()
	if err == nil {
		t.Fatalf("Should error when argument is not an ASCII int")
	}

	uCmd.SetArgs([]string{"10"})
	err = uCmd.Execute()
	if err == nil {
		t.Fatalf("Should error when ID is out of range")
	}

	uCmd.SetArgs([]string{"0"})
	err = uCmd.Execute()
	if err == nil {
		t.Fatalf("Should error when ID is 0")
	}

	strs := []string{"a"}
	for _, s := range strs {
		err := insert(db, TASKS_BUCKET, s, "")
		if err != nil {
			t.Fatalf("Failed to insert into db: %v", err)
		}
	}

	uCmd.SetArgs([]string{"1"})
	err = uCmd.Execute()
	if err == nil {
		t.Fatalf("Should error when no update flags are used")
	}

	// Make sure the -d updates a tasks description and -s can flip from incomplete -> complete
	updatedDesc := "updated"
	dFlag := fmt.Sprintf("-d=%s", updatedDesc)

	uCmd.SetArgs([]string{"1", dFlag, "-s"})
	err = uCmd.Execute()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	task, err := getTask(db, 1)
	if err != nil {
		t.Fatalf("Error retrieving task: %v ", err)
	}

	if task.Desc != updatedDesc || task.Status != STATUS.COMPLETE {
		t.Fatalf("Task did not update correctly. Expected: \"1. %s %s\". Got: \"1. %s %s\"", updatedDesc, STATUS.COMPLETE, task.Desc, task.Status)
	}

	// Make sure using tags with -d works && -s can flip from complete -> incomplete
	uCmd.SetArgs([]string{"1", "-d=+test hmm", "-s"})
	err = uCmd.Execute()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	t2, err := getTask(db, 1)
	if err != nil {
		t.Fatalf("Error retrieving task: %v", err)
	}

	if t2.Desc != "hmm" || t2.Status != STATUS.INCOMPLETE || t2.Tag != "test" {
		t.Fatalf("Task did not update correctly\nGot:%v", t2)
	}

	// Make sure passing a tag with no text to -d causes an error
	uCmd.SetArgs([]string{"1", "-d=+fail", "-s"})
	err = uCmd.Execute()
	if err == nil {
		t.Fatalf("Should error if -d is only passed a flag")
	}
}

func TestInsert(t *testing.T) {
	db, path := setup()
	defer teardown(db, path)

	strs := []string{"test", "prueba", "tesuto", "hoao"}
	expected := len(strs)
	for _, s := range strs {
		if err := insert(db, TASKS_BUCKET, s, ""); err != nil {
			t.Fatalf("Failed to insert into db: %v", err)
		}
	}
	count := getCount(db, TASKS_BUCKET)
	if count != expected {
		t.Fatalf("Have %d tasks, expected %d", count, expected)
	}
}

func TestGetCount(t *testing.T) {
	db, path := setup()
	defer teardown(db, path)

	strs := []string{"test", "prueba", "tesuto", "hoao"}
	remove := 2
	expected := len(strs) - remove
	count := 0

	for _, s := range strs {
		if err := insert(db, TASKS_BUCKET, s, ""); err != nil {
			t.Fatalf("Failed to insert into db: %v", err)
		}
	}

	updateErr := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(TASKS_BUCKET)
		if b == nil {
			t.Fatalf("tasks bucket does not exist")
		}

		c := b.Cursor()
		for i := 0; i < remove; i++ {
			k, _ := c.First()
			b.Delete(k)
		}
		return nil
	})
	check(updateErr)
	count = getCount(db, TASKS_BUCKET)
	if count != expected {
		t.Fatalf("Got %d tasks, expected %d", count, expected)
	}
}

func TestDeleteTask(t *testing.T) {
	db, path := setup()
	defer teardown(db, path)

	strs := []string{"test", "prueba", "tesuto", "hoao"}
	removeKeys := []int{1, 2}
	count := 0
	expected := len(strs) - len(removeKeys)

	for _, s := range strs {
		err := insert(db, TASKS_BUCKET, s, "")
		if err != nil {
			t.Fatalf("Failed to insert into db: %v", err)
		}
	}

	for _, k := range removeKeys {
		er := deleteKey(k, db, TASKS_BUCKET)
		if er != nil {
			t.Fatalf("Ran into an error: %v", er)
		}
	}

	count = getCount(db, TASKS_BUCKET)
	if count != expected {
		t.Fatalf("%d tasks exist, expected %d", count, expected)
	}
}

func TestDeleteMultipleTasks(t *testing.T) {
	db, path := setup()
	defer teardown(db, path)

	var bucketKeys []int
	var bucketValues []string
	strs := []string{"a", "b", "c", "d", "e", "f"}
	// Note: When `strs` are inserted into db they will be 1 indexed
	removeKeys := []int{1, 3, 5}
	expected := []string{"b", "d", "f"}

	for _, s := range strs {
		err := insert(db, TASKS_BUCKET, s, "")
		if err != nil {
			t.Fatalf("Failed to insert into db: %v", err)
		}
	}

	deleteKeys(removeKeys, db, TASKS_BUCKET)

	// Make sure remaining entires are in ascending order
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(TASKS_BUCKET)

		b.ForEach(func(k, v []byte) error {
			key := btoi(k)
			bucketKeys = append(bucketKeys, key)
			if key < bucketKeys[key-1] {
				t.Fatalf("Entries not in ascending order")
			}

			t := bToTask(v)
			bucketValues = append(bucketValues, t.Desc)
			return nil
		})
		return nil
	})

	// Make sure the correct tasks were deleted
	equal := reflect.DeepEqual(expected, bucketValues)
	if !equal {
		t.Fatalf("Tasks not in expected order.\n Expected: %v\n Got:%v", expected, bucketValues)
	}
}

func TestCompleteTask(t *testing.T) {
	db, path := setup()
	defer teardown(db, path)

	strs := []string{"test", "prueba", "tesuto", "hoao"}
	complete := []int{1, 2}
	expected := len(complete)
	var count int

	for _, s := range strs {
		err := insert(db, TASKS_BUCKET, s, "")
		if err != nil {
			t.Fatalf("Failed to insert into db: %v", err)
		}
	}

	for _, id := range complete {
		completeTask(id, db)
	}

	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(TASKS_BUCKET)
		return b.ForEach(func(k, v []byte) error {
			t := bToTask(v)
			if t.Status == STATUS.COMPLETE {
				count++
			}
			return nil
		})
	})

	if count != expected {
		t.Fatalf("%d tasks completed, expected %d", count, expected)
	}
}

func TestFinish(t *testing.T) {
	db, path := setup()
	defer teardown(db, path)

	strs := []string{"a", "b", "c", "d"}
	complete := []int{2, 3}
	expected := []string{"a", "d"}
	expectedArchive := []string{"b", "c"}

	for _, s := range strs {
		err := insert(db, TASKS_BUCKET, s, "")
		if err != nil {
			t.Fatalf("Failed to insert into db: %v", err)
		}
	}

	for _, id := range complete {
		completeTask(id, db)
	}

	finish(db)

	// make sure correct tasks were deleted & deleted tasks were added to archive
	var result []string
	var inArchive []string
	db.View(func(tx *bolt.Tx) error {
		remainingTasks := tx.Bucket(TASKS_BUCKET)
		archive := tx.Bucket(ARCHIVE_BUCKET)

		archive.ForEach(func(k, v []byte) error {
			t := bToTask(v)
			inArchive = append(inArchive, t.Desc)
			return nil
		})

		remainingTasks.ForEach(func(k, v []byte) error {
			t := bToTask(v)
			result = append(result, t.Desc)
			return nil
		})
		return nil
	})

	if !reflect.DeepEqual(expected, result) {
		t.Fatalf("Expected %v, Got %v", expected, result)
	}
	if !reflect.DeepEqual(expectedArchive, inArchive) {
		t.Fatalf("Error in archive: Expected %v, Got %v", expected, result)
	}
}

func TestFormatTasks(t *testing.T) {
	db, path := setup()
	defer teardown(db, path)

	strs := []string{"a", "b", "c"}
	complete := []int{2, 3}
	expected := `1: a 🔴
2: b ✅
3: c ✅`

	for _, s := range strs {
		err := insert(db, TASKS_BUCKET, s, "")
		if err != nil {
			t.Fatalf("Failed to insert into db: %v", err)
		}
	}

	for _, id := range complete {
		completeTask(id, db)
	}

	tp := getTasks(db, TASKS_BUCKET)
	result := formatTasks(tp)

	if result != expected {
		t.Logf("Expected len: %d, Got len: %d", len(expected), len(result))
		t.Fatalf("Expected %s, Got %s", expected, result)
	}
}

func TestParseTags(t *testing.T) {
	var tests = []struct {
		input,
		tag,
		output string
	}{
		{"no tag", "", "no tag"},
		{"+start house", "start", "house"},
		{"car +end", "end", "car"},
		{"+shop milk eggs + stuff", "shop", "milk eggs + stuff"},
		// pick the first tag from left, but still remove all tags
		{"+a +b +c", "a", ""},
		// trim whitespace
		{"  +trim me  ", "trim", "me"},
		// don't leave extra whitespace when removing tag
		{"a +middle c", "middle", "a c"},
		// only trim 1 whitespace preceding the tag
		{"d  +middle e", "middle", "d  e"},
	}

	for _, tt := range tests {
		testName := fmt.Sprintf("%v", tt.input)
		t.Run(testName, func(t *testing.T) {
			parsedTags, parsedStr := parseTags(tt.input)
			var tag string
			if len(parsedTags) >= 1 {
				tag = parsedTags[0]
			} else {
				tag = ""
			}

			if tag != tt.tag {
				t.Errorf("Wrong tag, Expected: %v, Got: %v", tt.tag, tag)
			}
			if parsedStr != tt.output {
				t.Errorf("Wrong output, Expected: %v, Got: %v", tt.output, parsedStr)
			}
		})
	}
}

func setup() (*bolt.DB, string) {
	path := filepath.Join(os.TempDir(), "task-test.db")
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	check(err)
	db.Update(func(tx *bolt.Tx) error {
		tx.CreateBucketIfNotExists(TASKS_BUCKET)
		tx.CreateBucketIfNotExists(ARCHIVE_BUCKET)
		return nil
	})
	return db, path
}

func teardown(db *bolt.DB, path string) {
	db.Update(func(tx *bolt.Tx) error {
		tx.DeleteBucket(TASKS_BUCKET)
		tx.DeleteBucket(ARCHIVE_BUCKET)
		return nil
	})
	db.Close()
	os.Remove(path)
}
