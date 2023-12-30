package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "task [command]",
	Short: "A CLI for managing your TODOs",
	// Long: ``
}

// Subcommands
var addCommand = &cobra.Command{
	Use:   "add [task]",
	Short: "Add a new task to your TODO list",
	Run: func(cmd *cobra.Command, args []string) {
		db := Connect()
		defer db.Close()

		tags, parsed := parseTags(strings.Join(args, " "))

		if parsed == "" {
			fmt.Println("Error: Empty task")
			return
		}

		var tag = ""
		if len(tags) >= 1 {
			// For now, only add the first tag to a task
			tag = tags[0]
		}

		err := insert(db, TASKS_BUCKET, parsed, tag)
		check(err)
		fmt.Printf("Added task: '%s'\n", parsed)
	},
}

var doCommand = &cobra.Command{
	Use:   "do [taskID]",
	Short: "Mark a task on your TODO list as complete",
	Run: func(cmd *cobra.Command, args []string) {
		db := Connect()
		defer db.Close()
		for _, v := range args {
			id, err := strconv.Atoi(v)
			if err != nil {
				println("Arguments should only be numbers")
				fmt.Printf("%s is not a number\n", v)
				os.Exit(1)
			}
			completeTask(id, db)
			fmt.Printf("Completed task %d\n", id)
		}
		fmt.Println()
		tp := getTasks(db)
		fmt.Println(formatTasks(tp))
	},
}

var updateCommand = &cobra.Command{
	Use:   "update [taskID] [-ds]",
	Short: "Update a task",
	Run: func(cmd *cobra.Command, args []string) {
		db := Connect()
		defer db.Close()

		// Make sure exactly 1 argument is passed in
		if len(args) == 0 {
			fmt.Printf("Must specify a task to update\n")
			return
		}
		if len(args) > 1 {
			fmt.Printf("Can only update one task at a time\n")
			return
		}

		// Make sure the argument is a number
		id, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Printf("Arguments should only be numbers\n")
			fmt.Printf("%s is not a number\n", args[0])
			return
		}

		// Make sure the input number is a valid taskID
		taskCount := getCount(db, TASKS_BUCKET)
		if id > taskCount {
			fmt.Printf("%d is out of range, %d tasks exist\n", id, taskCount)
			return
		}

		// Return early if there's no update to make
		if UpdatedDesc == "" && !UpdateStatus {
			fmt.Printf("Did not make any updates, try using a flag\n")
			return
		}

		t, _ := getTask(db, id)

		// Flip the task status
		if UpdateStatus {
			if t.Status == STATUS.COMPLETE {
				t.Status = STATUS.INCOMPLETE
			} else {
				t.Status = STATUS.COMPLETE
			}
		}

		// Update the task description
		if UpdatedDesc != "" {
			// Update the tag if a tag is present in the input
			tags, s := parseTags(UpdatedDesc)
			if s == "" {
				fmt.Printf("Must provide a task description\n")
				return
			}
			if len(tags) >= 1 {
				t.Tag = tags[0]
			}
			t.Desc = s
		}

		// Finally, update the task in the db
		if err := updateTask(db, id, t); err != nil {
			fmt.Printf("Ran into an error: %v", err)
			os.Exit(1)
		}

		fmt.Printf("Updated task %d\n", id)
		fmt.Println()

		// Print the updated tasks
		tp := getTasks(db)
		fmt.Println(formatTasks(tp))
	},
}

// Retrieve a task by key. Returns an error if the task bucket does not exist or if the key does not exist.
func getTask(db *bolt.DB, key int) (Task, error) {
	var t Task
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(TASKS_BUCKET)
		if b == nil {
			return errors.New("Task bucket does not exist")
		}

		buf := b.Get(itob(key))
		if buf == nil {
			return errors.New("Key does not exist")
		}

		t = bToTask(buf)
		return nil
	})
	return t, err
}

// Update a task in the db. Returns an error if the tasks bucket does not exist,
// if failed to marshal the task, or if failed to update the task in the db.
func updateTask(db *bolt.DB, taskId int, updated Task) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(TASKS_BUCKET)
		if b == nil {
			return errors.New("Tasks bucket does not exist")
		}

		t, jsonErr := json.Marshal(updated)
		if jsonErr != nil {
			return errors.New("Failed to marshal updated task")
		}

		return b.Put(itob(taskId), t)
	})
}

var listCommand = &cobra.Command{
	Use:   "list",
	Short: "List all of your incomplete tasks",
	Run: func(cmd *cobra.Command, args []string) {
		db := Connect()
		defer db.Close()

		input := strings.Join(args, " ")
		tasks := getTasks(db)
		if len(input) >= 1 {
			t, _ := parseTags(input)
			tasks = filterTasks(tasks, t)
		}
		fmt.Println(formatTasks(tasks))
	},
}

var finishCommand = &cobra.Command{
	Use:   "finish",
	Short: "Delete all completed tasks",
	Run: func(cmd *cobra.Command, args []string) {
		db := Connect()
		defer db.Close()

		err := finish(db)
		check(err)

		fmt.Printf("Deleted all completed tasks\n")
		tp := getTasks(db)
		fmt.Println(formatTasks(tp))
	},
}

// Filter out completed tasks from the `tasks` bucket
func finish(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(TASKS_BUCKET)
		archive, _ := tx.CreateBucketIfNotExists(ARCHIVE_BUCKET)
		if b == nil {
			return errors.New("No tasks exist")
		}

		var filtered [][]byte
		err := b.ForEach(func(k, v []byte) error {
			t := bToTask(v)

			if t.Status != STATUS.COMPLETE {
				filtered = append(filtered, v)
				return nil
			}
			// add the completed tasks to the archive bucket
			idx, _ := archive.NextSequence()
			return archive.Put(itob(int(idx)), v)
		})
		if err != nil {
			return err
		}

		tx.DeleteBucket(TASKS_BUCKET)
		newBucket, _ := tx.CreateBucket(TASKS_BUCKET)
		for _, v := range filtered {
			k, _ := newBucket.NextSequence()
			newBucket.Put(itob(int(k)), v)
		}
		return nil
	})
}

var clearCommand = &cobra.Command{
	Use:   "clear",
	Short: "Delete all tasks",
	Run: func(cmd *cobra.Command, args []string) {
		db := Connect()
		defer db.Close()

		db.Update(func(tx *bolt.Tx) error {
			tx.DeleteBucket(TASKS_BUCKET)
			return nil
		})
		fmt.Println("Deleted all tasks")
	},
}

var deleteCommand = &cobra.Command{
	Use:   "delete",
	Short: "Delete a task",
	Run: func(cmd *cobra.Command, args []string) {
		db := Connect()
		defer db.Close()

		var ids []int
		taskCount := getCount(db, TASKS_BUCKET)
		for _, s := range args {
			id, err := strconv.Atoi(s)
			if err != nil {
				println("Arguments should only be numbers")
				fmt.Printf("%s is not a number\n", args[0])
				os.Exit(1)
			}
			if id > taskCount {
				fmt.Printf("%d is out of range, only %d tasks exist\n", id, taskCount)
				return
			}
			ids = append(ids, id)
		}

		if len(ids) == 1 {
			er := deleteKey(ids[0], db, TASKS_BUCKET)
			check(er)
			fmt.Printf("Deleted task %d\n", ids[0])
			tp := getTasks(db)
			fmt.Println(formatTasks(tp))
			return
		}

		deleteKeys(ids, db, TASKS_BUCKET)
		for _, n := range ids {
			fmt.Println("Deleted Task ", n)
		}

		fmt.Println()
		tp := getTasks(db)
		fmt.Println(formatTasks(tp))
	},
}

var archiveCommand = &cobra.Command{
	Use:   "archive",
	Short: "View all previously completed tasks",
	Run: func(cmd *cobra.Command, args []string) {
		db := Connect()
		defer db.Close()

		if ClearArchive {
			db.Update(func(tx *bolt.Tx) error {
				er := tx.DeleteBucket(ARCHIVE_BUCKET)
				check(er)
				return nil
			})
			fmt.Println("Cleared the archive")
			return
		}

		db.View(func(tx *bolt.Tx) error {
			archive := tx.Bucket(ARCHIVE_BUCKET)
			if archive == nil || archive.Stats().KeyN == 0 {
				fmt.Println("Archive is empty, finish a task to add it to the archive")
				return nil
			}

			archive.ForEach(func(k, v []byte) error {
				var task Task
				json.Unmarshal(v, &task)

				idx := binary.BigEndian.Uint64(k)
				fmt.Printf("%d: %s\n", idx, task.Desc)
				return nil
			})
			return nil
		})
	},
}

var countCommand = &cobra.Command{
	Use:   "count",
	Short: "Print the number of existing tasks",
	Run: func(cmd *cobra.Command, args []string) {
		db := Connect()
		defer db.Close()
		num := getCount(db, TASKS_BUCKET)
		fmt.Printf("%d tasks\n", num)
	},
}

var tagsCommand = &cobra.Command{
	Use:   "tags",
	Short: "Print existing tags",
	Run: func(cmd *cobra.Command, args []string) {
		db := Connect()
		defer db.Close()

		var tags []string
		db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket(TASKS_BUCKET)
			return b.ForEach(func(k, v []byte) error {
				t := bToTask(v)
				if t.Tag != "" && !slices.Contains(tags, t.Tag) {
					tags = append(tags, t.Tag)
				}
				return nil
			})
		})
		fmt.Println(strings.Join(tags, ", "))
	},
}

// Flags
var ClearArchive bool
var ShowTags bool
var UpdatedDesc string
var UpdateStatus bool

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// add sub commands
	rootCmd.AddCommand(
		addCommand, doCommand,
		updateCommand, listCommand,
		finishCommand, clearCommand,
		archiveCommand, deleteCommand,
		countCommand, tagsCommand,
	)
	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.task-cli.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	archiveCommand.Flags().BoolVarP(&ClearArchive, "clear", "c", false, "Delete all archive entries")
	listCommand.Flags().BoolVarP(&ShowTags, "tag", "t", false, "Show tag associated with each task")
	updateCommand.Flags().StringVarP(&UpdatedDesc, "des", "d", "", "New task description. If a tag is present in the new description, the old tag will be replaced")
	updateCommand.Flags().BoolVarP(&UpdateStatus, "status", "s", false, "Flip the completion status of the task")
}

var TASKS_BUCKET = []byte("tasks")
var ARCHIVE_BUCKET = []byte("archive")
var STATUS = TaskStatus{"complete", "incomplete"}

type TaskStatus struct {
	COMPLETE   string
	INCOMPLETE string
}

type Task struct {
	Desc    string
	Status  string
	Created string
	Tag     string
}

type TaskPosition struct {
	task  Task
	dbKey int
}

func check(e error) {
	if e != nil {
		panic(e)
	}
	return
}

func Connect() *bolt.DB {
	hDir, e := os.UserHomeDir()
	check(e)

	// default is "/task"
	path := hDir + "/task"

	// creates the `task` dir if it doesn't exist
	dErr := os.MkdirAll(path, 0777)
	check(dErr)

	// default is "/tasks.db"
	db, err := bolt.Open(path+"/tasks.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
	check(err)
	return db
}

// Parse any tags in the form "+tag". Returns a slice of tags found and the original string with the
// tags removed. If no tags are found, returns an empty slice and the original string. Always returns ([]tags, s)
func parseTags(s string) ([]string, string) {
	// Matches substrings in the form "+text" Captures "text".
	re := regexp.MustCompile(`\+([^ ]+)`)
	var tags []string
	parsed := s

	// match[0] is the entire match, match[1] is the capture group
	matches := re.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		if m != nil && len(m) >= 2 {
			tags = append(tags, m[1])

			// remove extra whitespace when a tag is the the middle of a string. ex "a +b c" -> "a c"
			spaceBefore := " " + m[0]
			if strings.Contains(s, spaceBefore) {
				b, a, _ := strings.Cut(parsed, spaceBefore)
				parsed = b + a
			} else {
				parsed = strings.Replace(parsed, m[0], "", 1)
			}
		}
	}
	return tags, strings.TrimSpace(parsed)
}

// Opens an Update transaction with `db`, creates a Task from `s` and inserts the task into `bucket`
func insert(db *bolt.DB, bucket []byte, s string, tag string) error {
	err := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucket)
		if err != nil {
			return err
		}

		// create an id and convert it to a []byte
		id, _ := b.NextSequence()
		byteId := itob(int(id))

		task := Task{
			Desc:    s,
			Status:  STATUS.INCOMPLETE,
			Created: time.Now().String(),
			Tag:     tag,
		}

		// Marshal Task data into bytes.
		buf, err := json.Marshal(task)
		if err != nil {
			return err
		}
		return b.Put(byteId, buf)

	})
	return err
}

// Returns a slice containing all tasks in the database along with their respective positions.
func getTasks(db *bolt.DB) []TaskPosition {
	var tasks []TaskPosition
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(TASKS_BUCKET)
		return b.ForEach(func(k, v []byte) error {
			t := bToTask(v)
			tasks = append(tasks, TaskPosition{
				task:  t,
				dbKey: btoi(k),
			})
			return nil
		})
	})
	return tasks
}

// Filter tasks by tag. Returns a slice of tasks whose tag is present in `include`.
func filterTasks(tp []TaskPosition, include []string) []TaskPosition {
	// no tags to filter by, return tp
	if len(include) == 0 {
		return tp
	}

	var filtered []TaskPosition
	// "none" tag can be used to filter tasks with no tag
	includeNoTag := slices.Contains(include, "none")
	for _, t := range tp {
		if t.task.Tag == "" && includeNoTag {
			filtered = append(filtered, t)
		}
		if slices.Contains(include, t.task.Tag) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// Format the tasks in db, return the formatted string
func formatTasks(tp []TaskPosition) string {
	var formatted []string
	for _, t := range tp {
		s := "🔴"
		if t.task.Status == STATUS.COMPLETE {
			s = "✅"
		}
		if ShowTags {
			formatted = append(formatted, fmt.Sprintf("%d: %s: %s %s", t.dbKey, t.task.Tag, t.task.Desc, s))
			continue
		}
		formatted = append(formatted, fmt.Sprintf("%d: %s %s", t.dbKey, t.task.Desc, s))
	}
	return strings.Join(formatted, "\n")
}

// Opens a View transaction with `db` and returns the number of entries in `bucket`
func getCount(db *bolt.DB, bucket []byte) int {
	var count int
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			count = 0
			return nil
		}
		count = b.Stats().KeyN
		return nil
	})
	return count
}

// Opens an Update transaction with `db` and deletes the entry from `bucket`
// whose key matches `key`. Returns an error if the bucket does not exist, failed to delete an entry
// or failed to renumber the remaining entries
func deleteKey(k int, db *bolt.DB, bucket []byte) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return errors.New(fmt.Sprintf("Could not find the `%s` bucket", string(bucket)))
		}
		err := b.Delete(itob(k))
		if err != nil {
			return err
		}
		return renumberEntires(b)
	})
}

// Remove the specified keys by filtering the bucket, deleting the bucket and
// inserting the filtered items into a new bucket with the same name.
// O(n), filter n items, insert n items
func deleteKeys(toDelete []int, db *bolt.DB, bucket []byte) {
	db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			fmt.Printf("`%s` bucket does not exist", string(bucket))
			os.Exit(1)
		}

		var filtered [][]byte
		b.ForEach(func(k, v []byte) error {
			ignore := slices.Contains(toDelete, btoi(k))
			if !ignore {
				filtered = append(filtered, v)
			}
			return nil
		})
		tx.DeleteBucket(bucket)

		// Create a new bucket, insert the filtered tasks and renumber
		newBucket, _ := tx.CreateBucket(bucket)
		for _, t := range filtered {
			k, _ := newBucket.NextSequence()
			newBucket.Put(itob(int(k)), t)
		}
		return renumberEntires(newBucket)
	})
}

// Update the specified tasks status to `completed`
func completeTask(taskID int, db *bolt.DB) {
	db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(TASKS_BUCKET)
		if b == nil {
			fmt.Println("Could not find a tasks database")
			os.Exit(1)
		}

		byteId := itob(taskID)

		val := b.Get(byteId)
		if val == nil {
			fmt.Printf("Task %d does not exist\n", taskID)
			os.Exit(1)
		}

		var t Task

		json.Unmarshal(val, &t)
		if t.Status == STATUS.COMPLETE {
			fmt.Printf("You already finished task %d\n", taskID)
			return nil
		}

		t.Status = STATUS.COMPLETE
		updatedTask, err := json.Marshal(t)
		check(err)

		// update the `tasks` bucket with the completed task
		b.Put(byteId, updatedTask)

		return nil
	})

}

// Renumber bucket entries in ascending order.
// Especially useful after deleting an entry in the middle of the bucket
func renumberEntires(bucket *bolt.Bucket) error {
	// can ignore errors if this is called in an Update() call:
	// Delete() can't fail in an Update() call,
	// Put() shouldn't fail since the items already existed in the db
	idx := 0
	bucket.ForEach(func(k, v []byte) error {
		idx++
		bucket.Delete(k)
		bucket.Put(itob(idx), v)
		return nil
	})
	// update the Sequence to match the number of remaining entries
	er := bucket.SetSequence(uint64(idx))
	return er
}

// Convert an int to a byte slice
func itob(v int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

// Convert a byte slice to an int
func btoi(b []byte) int {
	return int(binary.BigEndian.Uint64(b))
}

// Unmarshal a byte slice to a Task struct
func bToTask(b []byte) Task {
	var task Task
	err := json.Unmarshal(b, &task)
	check(err)
	return task
}
