package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
func newAddCmd(db *bolt.DB, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "add [task]",
		Short: "Add a new task to your TODO list",
		Run: func(cmd *cobra.Command, args []string) {
			tags, parsed := parseTags(strings.Join(args, " "))

			if parsed == "" {
				fmt.Fprintf(out, "Error: Empty task\n")
				return
			}

			var tag = ""
			if len(tags) >= 1 {
				// For now, only add the first tag to a task
				tag = tags[0]
			}

			err := insert(db, TASKS_BUCKET, parsed, tag)
			check(err)
			fmt.Fprintf(out, "Added task: '%s'\n", parsed)

		},
	}
}

func newDoCmd(db *bolt.DB, out io.Writer) *cobra.Command {
	doCmd := &cobra.Command{
		Use:   "do [taskID]",
		Short: "Mark a task on your TODO list as complete",
		Run: func(cmd *cobra.Command, args []string) {
			var keys []int
			for _, v := range args {
				id, err := strconv.Atoi(v)
				if err != nil {
					fmt.Fprintln(out, "Arguments should only be numbers")
					fmt.Fprintf(out, "%s is not a number\n", v)
					os.Exit(1)
				}
				keys = append(keys, id)
				completeTask(id, db)
				fmt.Fprintf(out, "Completed task %d\n", id)
			}
			if DeleteOnDo {
				deleteKeys(keys, db, TASKS_BUCKET)
			}
			fmt.Fprintln(out)
			tp := getTasks(db, TASKS_BUCKET)
			fmt.Fprintln(out, formatTasks(tp))
		},
	}
	doCmd.Flags().BoolVarP(&DeleteOnDo, "finish", "f", false, "Complete and finish the specified tasks")
	return doCmd
}

func newUpdateCmd(db *bolt.DB, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [taskID] [-ds]",
		Short: "Update a task",
		// SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Setting this to true at the start of the RunE instead of the cmd itself
			// ensures that flag parsing errors will still display the usage message
			cmd.SilenceUsage = true

			// Make sure exactly 1 argument is passed in
			if len(args) != 1 {
				return errors.New("Must specify a single task to update")
			}

			// Make sure the argument is an int
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return errors.New(fmt.Sprintf("Argument should be an integer\n\"%s\" is not an integer", args[0]))
			}

			// Make sure the input number is a valid taskID
			taskCount := getCount(db, TASKS_BUCKET)
			if id > taskCount || id == 0 {
				return errors.New(fmt.Sprintf("Invalid task ID, %d tasks exist", taskCount))
			}

			// Return early if there's no update to make
			if UpdatedDesc == "" && !UpdateStatus {
				cmd.SilenceUsage = false
				return errors.New("Did not make any updates, try using a flag")
			}

			t, _ := getTask(db, id)

			// Flip the task status
			if UpdateStatus {
				if t.Status == STATUS.COMPLETE {
					t.Status = STATUS.INCOMPLETE
					t.Completed = ""
				} else {
					t.Status = STATUS.COMPLETE
					t.Completed = time.Now().Format(RFC3339)
				}
			}

			// Update the task description
			if UpdatedDesc != "" {
				// Update the tag if a tag is present in the input
				tags, s := parseTags(UpdatedDesc)
				if s == "" {
					return errors.New("Must provide a task description")
				}
				if len(tags) >= 1 {
					t.Tag = tags[0]
				}
				t.Desc = s
			}

			// Finally, update the task in the db
			if err := updateTask(db, id, t); err != nil {
				return err
			}

			fmt.Fprintf(out, "Updated task %d\n", id)

			// Print the updated tasks
			tp := getTasks(db, TASKS_BUCKET)
			fmt.Fprintln(out, formatTasks(tp))
			return nil
		},
	}
	cmd.Flags().StringVarP(&UpdatedDesc, "des", "d", "", "New task description. If a tag is present in the new description, the old tag will be replaced")
	cmd.Flags().BoolVarP(&UpdateStatus, "status", "s", false, "Flip the completion status of the task")
	return cmd
}

func newListCmd(db *bolt.DB, out io.Writer) *cobra.Command {
	lCmd := &cobra.Command{
		Use:   "list -[te]",
		Short: "List all of your incomplete tasks",
		Run: func(cmd *cobra.Command, args []string) {
			var exclude []string
			var include []string

			exclude = strings.Split(ExcludeTags, ",")
			// Avoids buggy behavior when user inputs "-e" or "-e="
			if len(exclude) == 1 && exclude[0] == "" {
				exclude = []string{}
			}

			input := strings.Join(args, " ")
			if len(input) >= 1 {
				include, _ = parseTags(input)
			}

			if len(include) > 0 && len(exclude) > 0 {
				fmt.Fprintln(out, "Can't use tag filtering in combination with exclude flag")
				return
			}

			tasks := getTasks(db, TASKS_BUCKET)
			tasks = filterTasks(tasks, include, exclude)
			fmt.Fprintln(out, formatTasks(tasks))
		},
	}
	lCmd.Flags().BoolVarP(&ShowTags, "tag", "t", false, "Show tag associated with each task")
	lCmd.Flags().StringVarP(&ExcludeTags, "exclude", "e", "", "Exclude tasks with listed tags. The tags should be comma seperated. Example: -e=tag1,tag2,tag3")
	return lCmd
}

func newFinishCmd(db *bolt.DB, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "finish",
		Short: "Delete all completed tasks",
		Run: func(cmd *cobra.Command, args []string) {
			err := finish(db)
			check(err)

			fmt.Fprintf(out, "Deleted all completed tasks\n")
			tp := getTasks(db, TASKS_BUCKET)
			fmt.Fprintln(out, formatTasks(tp))
		},
	}
}

func newClearCmd(db *bolt.DB, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Delete all tasks",
		Run: func(cmd *cobra.Command, args []string) {
			db.Update(func(tx *bolt.Tx) error {
				tx.DeleteBucket(TASKS_BUCKET)
				return nil
			})
			fmt.Fprintln(out, "Deleted all tasks")
		},
	}
}

func newDeleteCmd(db *bolt.DB, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "delete",
		Short: "Delete a task",
		Run: func(cmd *cobra.Command, args []string) {
			var ids []int
			taskCount := getCount(db, TASKS_BUCKET)

			for _, s := range args {
				id, err := strconv.Atoi(s)
				if err != nil {
					fmt.Fprintln(out, "Arguments should only be numbers")
					fmt.Fprintf(out, "%s is not a number\n", args[0])
					os.Exit(1)
				}
				if id > taskCount {
					fmt.Fprintf(out, "%d is out of range, only %d tasks exist\n", id, taskCount)
					return
				}
				ids = append(ids, id)
			}

			if len(ids) == 1 {
				er := deleteKey(ids[0], db, TASKS_BUCKET)
				check(er)
				fmt.Fprintf(out, "Deleted task %d\n", ids[0])
				tp := getTasks(db, TASKS_BUCKET)
				fmt.Fprintln(out, formatTasks(tp))
				return
			}

			deleteKeys(ids, db, TASKS_BUCKET)
			for _, n := range ids {
				fmt.Fprintln(out, "Deleted Task ", n)
			}

			fmt.Fprintln(out)
			tp := getTasks(db, TASKS_BUCKET)
			fmt.Fprintln(out, formatTasks(tp))
		},
	}
}

func newArchiveCmd(db *bolt.DB, out io.Writer) *cobra.Command {
	arCmd := &cobra.Command{
		Use:   "archive -[c]",
		Short: "View all previously completed tasks",
		Run: func(cmd *cobra.Command, args []string) {
			if ClearArchive {
				db.Update(func(tx *bolt.Tx) error {
					er := tx.DeleteBucket(ARCHIVE_BUCKET)
					check(er)
					return nil
				})
				fmt.Fprintln(out, "Cleared the archive")
				return
			}

			db.View(func(tx *bolt.Tx) error {
				archive := tx.Bucket(ARCHIVE_BUCKET)
				if archive == nil || archive.Stats().KeyN == 0 {
					fmt.Fprintln(out, "Archive is empty, finish a task to add it to the archive")
					return nil
				}

				archive.ForEach(func(k, v []byte) error {
					var task Task
					json.Unmarshal(v, &task)

					idx := binary.BigEndian.Uint64(k)
					fmt.Fprintf(out, "%d: %s\n", idx, task.Desc)
					return nil
				})
				return nil
			})
		},
	}
	arCmd.Flags().BoolVarP(&ClearArchive, "clear", "c", false, "Delete all archive entries")
	return arCmd
}

func newStatsCmd(db *bolt.DB, out io.Writer) *cobra.Command {
	sCmd := &cobra.Command{
		Use:   "stats",
		Short: "See statistics on your task completion",
		Run: func(cmd *cobra.Command, args []string) {
			// Define the expected date format
			mmddyyyy := "01/02/2006"
			var startDate time.Time
			var endDate time.Time
			var mustInputStart bool
			var err error

			// Attempt to parse using mm/dd/yyy format
			endDate, err = time.Parse(mmddyyyy, EndTime)
			if err == nil {
				mustInputStart = true
			} else {
				// Defaults to now
				endDate = time.Now()
			}

			// Attempt to parse using mm/dd/yyy format
			startDate, err = time.Parse(mmddyyyy, StartTime)
			if err != nil && mustInputStart {
				// User input an end but no start
				fmt.Fprintln(out, "Must specify a start date")
				return
			}
			if err != nil {
				// Defaults to last 24hrs
				startDate, err = time.Parse(RFC3339, time.Now().Add(-24*time.Hour).Format(RFC3339))
				if err != nil {
					fmt.Fprintln(out, "Error parsing start date:", err)
					return
				}
			}

			if endDate.Before(startDate) {
				fmt.Fprintln(out, "Error: End date occured prior to the Start date")
				return
			}

			if OnDay != "" {
				day, err := time.Parse(mmddyyyy, OnDay)
				if err != nil {
					fmt.Fprintln(out, "Error parsing date:", err)
					return
				}
				startDate = day
				endDate = day
			}

			// If the user inputs the same start and end date, then set the end date to the last tick (12:59) of that day.
			if startDate.Equal(endDate) {
				endDate = lastTick(endDate)
			}

			var filtered []TaskPosition
			tasks := getTasks(db, ARCHIVE_BUCKET)
			for _, t := range tasks {
				completed, err := time.Parse(RFC3339, t.task.Completed)
				if err != nil {
					fmt.Fprintln(out, "Error parsing completed date:", err)
					return
				}

				if completed.After(startDate) && completed.Before(endDate) {
					filtered = append(filtered, t)
					// Useful for debugging
					// fmt.Fprintln(out, completed)
				}
			}

			if ShowCompleted {
				fmt.Fprintln(out, formatTasks(filtered))
			}
			sy, sm, sd := startDate.Date()
			ey, em, ed := endDate.Date()
			numCompleted := max(len(filtered), 0)

			fmt.Fprintf(out, "\nYou completed %d tasks from %d/%d/%d to %d/%d/%d\n", numCompleted, sm, sd, sy, em, ed, ey)
			if ShowAverage {
				diff := endDate.Sub(startDate)
				numDays := diff.Hours() / 24
				avg := float64(numCompleted) / numDays
				fmt.Fprintf(out, "Average: %.1f/day\n", avg)
			}
		},
	}
	sCmd.Flags().StringVarP(&StartTime, "start", "s", "", "mm/dd/yyyy formated date to specify the start period")
	sCmd.Flags().StringVarP(&EndTime, "end", "e", "", "mm/dd/yyyy formated date to specify the end window")
	sCmd.Flags().StringVarP(&OnDay, "on", "o", "", "mm/dd/yyyy formated date. Shorthand for setting the start and end date to the same day. Note that the on flag cannot be used with the start or end flags")
	sCmd.Flags().BoolVarP(&ShowCompleted, "verbose", "v", false, "Show the completed tasks")
	sCmd.Flags().BoolVarP(&ShowAverage, "average", "a", false, "Show the average tasks completed/day")
	sCmd.MarkFlagsMutuallyExclusive("start", "on")
	sCmd.MarkFlagsMutuallyExclusive("end", "on")
	return sCmd
}

func newCountCmd(db *bolt.DB, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "count",
		Short: "Print the number of existing tasks",
		Run: func(cmd *cobra.Command, args []string) {
			num := getCount(db, TASKS_BUCKET)
			fmt.Fprintf(out, "%d tasks\n", num)
		},
	}
}

func newTagsCmd(db *bolt.DB, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "tags",
		Short: "Print existing tags",
		Run: func(cmd *cobra.Command, args []string) {
			tags := getAllTags(db)
			fmt.Fprintln(out, strings.Join(tags, ","))
		},
	}
}

func getAllTags(db *bolt.DB) []string {
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
	return tags
}

// Flags
// $ archive
var ClearArchive bool

// $ list
var ShowTags bool
var ExcludeTags string

// $ update
var UpdatedDesc string
var UpdateStatus bool

// $ do
var DeleteOnDo bool

// $ stats
var StartTime string
var EndTime string
var OnDay string
var ShowCompleted bool
var ShowAverage bool

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

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.task-cli.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

}

var TASKS_BUCKET = []byte("tasks")
var ARCHIVE_BUCKET = []byte("archive")
var STATUS = TaskStatus{"complete", "incomplete"}

var RFC3339 = "2006-01-02T15:04:05Z07:00"

type TaskStatus struct {
	COMPLETE   string
	INCOMPLETE string
}

type Task struct {
	Desc      string
	Status    string
	Created   string
	Completed string
	Tag       string
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
			Desc:      s,
			Status:    STATUS.INCOMPLETE,
			Created:   time.Now().Format(RFC3339),
			Completed: "",
			Tag:       tag,
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
func getTasks(db *bolt.DB, bucket []byte) []TaskPosition {
	var tasks []TaskPosition
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
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

// Filter tasks by tag. Returns a slice of tasks whose tag is present in `include`.
// One on the []string must be empty i.e. can only include or exclude, can't do both.
func filterTasks(tp []TaskPosition, include, exclude []string) []TaskPosition {
	// no tags to filter by, return tp
	if len(include) == 0 && len(exclude) == 0 {
		return tp
	}

	var filtered []TaskPosition

	// First filter out any unwanted tasks
	excludeNoTag := slices.Contains(exclude, "none")
	for _, t := range tp {
		if slices.Contains(exclude, t.task.Tag) {
			continue
		}
		if t.task.Tag == "" && excludeNoTag {
			continue
		}
		filtered = append(filtered, t)
	}

	var finalFilter []TaskPosition

	// "none" tag can be used to filter tasks with no tag
	includeNoTag := slices.Contains(include, "none")
	for _, t := range filtered {
		if t.task.Tag == "" && includeNoTag {
			finalFilter = append(finalFilter, t)
		}
		if slices.Contains(include, t.task.Tag) {
			finalFilter = append(finalFilter, t)
		}
	}
	if len(include) > 0 {
		return finalFilter
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
		t.Completed = time.Now().Format(RFC3339)
		updatedTask, err := json.Marshal(t)
		check(err)

		// update the `tasks` bucket with the completed task
		b.Put(byteId, updatedTask)

		return nil
	})

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

// Returns the last tick of the provided time in the form:
// yyyy-mm-dd 23:59:59.999999999
func lastTick(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d+1, 0, 0, 0, -1, t.Location())
}
