package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"aesc-client/login"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("get homedir: %v", err)
	}
	credPath := filepath.Join(home, ".aesc_login")
	name, pass, err := login.ReadLogpass(credPath)
	if err != nil {
		log.Fatalf("read credentials: %v", err)
	}
	client, err := login.NewClient()
	if err != nil {
		log.Fatalf("new client: %v", err)
	}
	status, err := login.TryLogin(client, "http://server.aesc.msu.ru", "/cs/login", name, pass)
	if err != nil {
		log.Fatalf("login failed: %v", err)
	}
	fmt.Println("Login status:", status)
}
