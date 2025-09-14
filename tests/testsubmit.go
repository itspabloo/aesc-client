package test

import (
	"fmt"
	"os"
	"path/filepath"

	"aesc-client/login"
	"aesc-client/parse"
	"aesc-client/submit"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "get homedir: %v\n", err)
		os.Exit(2)
	}
	credPath := filepath.Join(home, ".aesc_test_login")

	name, pass, err := login.ReadLogpass(credPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read credentials: %v\n", err)
		os.Exit(2)
	}

	client, err := login.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "new client: %v\n", err)
		os.Exit(2)
	}

	_, err = login.TryLogin(client, "http://server.aesc.msu.ru", "/cs/login", name, pass)
	if err != nil {
		fmt.Fprintf(os.Stderr, "login failed: %v\n", err)
		os.Exit(2)
	}

	resp, err := client.Get("http://server.aesc.msu.ru/cs/motd")
	if err != nil {
		fmt.Fprintf(os.Stderr, "GET /cs/motd failed: %v\n", err)
		os.Exit(2)
	}
	defer resp.Body.Close()

	contests, err := parse.ParseContests(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ParseContests failed: %v\n", err)
		os.Exit(2)
	}
	if len(contests) == 0 {
		fmt.Fprintln(os.Stderr, "no contests found")
		os.Exit(1)
	}

	resp1, err := client.Get("http://server.aesc.msu.ru" + contests[0].URL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GET %s failed: %v\n", contests[0].URL, err)
		os.Exit(2)
	}
	defer resp1.Body.Close()

	tasks, err := parse.ParseProblems(resp1.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ParseTasks failed: %v\n", err)
		os.Exit(2)
	}
	if len(tasks) == 0 {
		fmt.Fprintln(os.Stderr, "no tasks found")
		os.Exit(1)
	}

	actionURL := "http://server.aesc.msu.ru" + tasks[0].URL
	filePath := "your_solution_here.cpp"

	err = submit.SubmitSolution(client, actionURL, filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "submit failed: %v\n", err)
		os.Exit(2)
	}

	fmt.Println("solution submitted successfully")
}

