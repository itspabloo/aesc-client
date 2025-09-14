package submit

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func detectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".cpp", ".cc", ".cxx":
		return "g++0x"
	case ".c":
		return "gcc"
	case ".py":
		return "python3.2"
	case ".pas":
		return "pabc"
	case ".cs":
		return "mono-cs"
	case ".java":
		return "kylix"
	case ".txt":
		return "txt"
	default:
		return "g++0x"
	}
}

func SubmitSolution(client *http.Client, actionURL, filePath string) error {
	lang := detectLanguage(filePath)

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("solutionSource", filepath.Base(filePath))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return err
	}

	_ = writer.WriteField("compileWith", lang)
	_ = writer.WriteField("sourceCharset", "cp1251")

	err = writer.Close()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", actionURL, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("submit failed: %s", resp.Status)
	}

	return nil
}

