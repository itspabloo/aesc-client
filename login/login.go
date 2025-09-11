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
	if len(file) < 2 {
		return "", "", errors.New("wrong .aesc_login format: file should contain at least two lines: login and password")
	}
	login = strings.TrimSpace(file[0])
	password = strings.TrimSpace(file[1])
	if login == "" || password == "" {
		return "", "", errors.New("wrong .aesc_login format: login or password is empty")
	}
	return login, password, nil
}

func NewClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	c := &http.Client{
		Jar: jar,
		Timeout: 30 * time.Second,
	}
	return c, nil
}

func TryLogin(client *http.Client, base, loginPath, name, password string) (string, error) {
	loginURL := strings.TrimRight(base, "/") + loginPath
	form := url.Values{}
	form.Set("name", name)
	form.Set("password", password)

	req, err := http.NewRequest("POST", loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", loginURL)
	req.Header.Set("User-Agent", "msu-client/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("perform login request: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return resp.Status, fmt.Errorf("login failed: %s", resp.Status)
	}
	return resp.Status, nil
}

func SaveCookies(jar http.CookieJar, baseURL, outPath string) error {
	if jar == nil {
		return errors.New("nil cookie jar")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	cookies := jar.Cookies(u)
	fdir := filepath.Dir(outPath)
	if fdir != "." {
		err := os.MkdirAll(fdir, 0o700)
		if err != nil {
			return fmt.Errorf("mkdir %s: %w", fdir, err)
		}
	}
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", outPath, err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, c := range cookies {
		if c.Name == "" {
			continue
		}
		line := fmt.Sprintf("%s\t%s\n", c.Name, c.Value)
		_, err := w.WriteString(line)
		if err != nil {
			return fmt.Errorf("write cookie: %w", err)
		}
	}
	err = w.Flush()
	if err != nil {
		return fmt.Errorf("flush cookies file: %w", err)
	}
	return nil
}

func LoadSessionCookies(jar http.CookieJar, baseURL, inPath string) error {
	if jar == nil {
		return errors.New("nil cookie jar")
	}
	b, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("read cookie file: %w", err)
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	lines := strings.Split(string(b), "\n")
	cookies := []*http.Cookie{}
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		parts := strings.SplitN(ln, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		cookies = append(cookies, &http.Cookie{Name: parts[0], Value: parts[1], Path: "/"})
	}
	jar.SetCookies(u, cookies)
	return nil
}
