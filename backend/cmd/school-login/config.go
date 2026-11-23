package main

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strings"
)

const schoolLoginDir = "cmd/school-login"

func loadSchoolConfig() (site, username, password string, err error) {
	const path = schoolLoginDir + "/.env"
	file, err := os.Open(path)
	if err != nil {
		return "", "", "", fmt.Errorf("从 backend 目录运行，并检查 %s: %w", path, err)
	}
	defer file.Close()

	values := make(map[string]string, 3)
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return "", "", "", fmt.Errorf("%s 第 %d 行应为 KEY=VALUE", path, lineNumber)
		}
		values[strings.TrimSpace(key)] = value
	}
	if err := scanner.Err(); err != nil {
		return "", "", "", fmt.Errorf("读取 %s: %w", path, err)
	}
	for _, key := range []string{"SCHOOL_URL", "SCHOOL_USERNAME", "SCHOOL_PASSWORD"} {
		if values[key] == "" || (key != "SCHOOL_PASSWORD" && strings.TrimSpace(values[key]) == "") {
			return "", "", "", fmt.Errorf("请在 %s 填写 %s", path, key)
		}
	}
	site = strings.TrimRight(strings.TrimSpace(values["SCHOOL_URL"]), "/")
	parsed, err := url.Parse(site)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Path != "" || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", "", fmt.Errorf("%s 的 SCHOOL_URL 应为 HTTPS 网站根地址", path)
	}
	return site, strings.TrimSpace(values["SCHOOL_USERNAME"]), values["SCHOOL_PASSWORD"], nil
}
