# Task CLI 
---
Manage all your tasks without ever leaving the command line.

https://github.com/allmtz/task-cli/assets/107075637/4b00781d-03b5-4b60-b7fc-9ddd3c229692

> This example uses the `task` alias. See [here](#create-an-alias) for more details on creating aliases.

Flow
---
The goal of this project is to allow the user to quickly and seamlessly jot down tasks/notes as soon as they appear. To this end I highly recommend using a hotkey to open the terminal. I've found ``cmd + ` ``   to be a great choice for this.

No matter how you decide to use `task-cli`, having instant access to the terminal and a fast way to jot down notes enables you to say focused longer. 

### Features
---
Use tags as a way to organize your tasks
```shell
task add +groceries buy milk
```

Filter tasks by tags
```shell
task list +groceries
```

List tasks with no tag
```shell
task list +none
```

Get a list of all your tags
```shell
task tags
```

An archive keeps track of all finished tasks
```shell
task archive
```

Track your productivity and see how many tasks you've completed in the last day
```shell
task stats
```

See [Subcommands](#subcommands) for a full list of available subcommands and flags.

### Installation
---
Both methods require Go to be installed.

Using Go
```bash
go install github.com/allmtz/task-cli@latest
```

Using Github 
```bash
git clone https://github.com/allmtz/task-cli.git && cd ./task-cli && go install .
```

### Create an alias
---
The default command to use the program is `task-cli`. To simplify usage, I recommend creating an alias in your `.bashrc` file. 

To create an alias, replace `YOUR_ALIAS` with the command you want to use to run `task-cli` and then execute the script below.

```shell
echo alias YOUR_ALIAS="task-cli" >> ~/.bashrc && source ~/.bashrc
```

### Subcommands 
- `add [task]` 
	- Add a task
	- Wrap your `task` in quotes if you need to use special characters
	- Use the `+tag` syntax anywhere in your task to add a tag to it
- `list -[te]`
	- List tasks
	- Use `-t` to print tasks along with their tag
	- Use `-e=tag` to exclude tasks with a given `tag`
	- Use the `+tag` syntax to only list tasks with the provided `tag`
- `do [ID] -[f]`
	- Mark a task as completed
	- Use `-f` to complete and finish the task in one step
- `update [ID] -[ds]`
	- Use `-d=[new_description]` to update the description of a task. Any tags present in the `new_description` will overwrite previous tags
	- Use `-s` to flip the completion status of a task
- `delete [ID]`
	- Delete a task. It will not be added to the archive
- `count`
	- Print the number of existing tasks
- `tags`
	- Print all existing tags
- `finish`
	- Remove all completed tasks and add them to the archive
- `clear`
	- Delete all tasks regardless of completion status. Note, deleted tasks will not be added to the archive
- `archive -[c]` 
	- View all finished tasks
	- Use `-c` to permanently delete all archive entries. Use with caution.
- `stats -[aseo]`
	- Print the number of completed tasks in the last 24 hours
	- The time period for stats to look at can be customized by using `-s=[date]` to specify the start date and `-e=[date]` to specify the end date. `date` must be in the format mm/dd/yyy
	- Use `-a` to also print the tasks completed per day for a given time period
	- Use `-o=[date]` to print the stats for the provided `date`
