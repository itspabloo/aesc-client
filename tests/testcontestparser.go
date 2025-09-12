package test

import (
	"fmt"
	"os"
	"path/filepath"

	"aesc-client/login"
	"aesc-client/parse"
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
		fmt.Fprintf(os.Stderr, "GET /cs failed: %v\n", err)
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

	for i := range len(contests) {
		fmt.Printf("%d. %s -> %s\n", i+1, contests[i].Name, contests[i].URL)
	}
}
