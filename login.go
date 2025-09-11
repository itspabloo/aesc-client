package login

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"io"
	"errors"
	"bufio"
	"strings"
)

func ReadLogpass(logpassPath string) (login string, password string, err error) {
	fin, err := os.Open(logpassPath)
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", logpassPath, err)
	}
	defer fin.Close()
	s := bufio.NewScanner(fin)
	file := []string{}
	for s.Scan() {
		file = append(file, s.Text())
		if len(file) >= 2 {
			break
		}
	}
	err = s.Err()
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", logpassPath, err)
	}
	if len(file) != 2 {
		return "", "", errors.New("wrong .aesc_login format: file should contain at least two lines: login and password")
	}
	login = strings.TrimSpace(file[0])
	password = strings.TrimSpace(file[1])
	if login == "" || password == "" {
		return "", "", errors.New("wrong .aesc_login format: login or password is empty")
	}
	return login, password, nil
}
