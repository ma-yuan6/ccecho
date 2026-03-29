package viewer

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var idxPattern = regexp.MustCompile(`(\d+)`)

// parseDetailPath 将 `/api/detail/{session}/{idx}` 去前缀后的路径拆成会话名和数字索引。
func parseDetailPath(path string) (string, int, bool) {
	parts := splitPath(path)
	if len(parts) != 2 {
		return "", 0, false
	}
	idx, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false
	}
	return parts[0], idx, true
}

// requestFilePath 返回指定请求条目的原始请求文件路径。
func requestFilePath(dir string, idx int) string {
	return filepath.Join(dir, fmt.Sprintf("request%d.json", idx))
}

// previousRequestFilePath 返回上一条请求文件路径；不存在时返回空字符串。
func previousRequestFilePath(dir string, idx int) string {
	if idx <= 1 {
		return ""
	}
	path := requestFilePath(dir, idx-1)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// responseStreamFilePath 返回指定条目的流式响应落盘路径。
func responseStreamFilePath(dir string, idx int) string {
	return filepath.Join(dir, fmt.Sprintf("response%d.stream", idx))
}

// responseJSONFilePath 返回指定条目的最终 JSON 响应落盘路径。
func responseJSONFilePath(dir string, idx int) string {
	return filepath.Join(dir, fmt.Sprintf("response%d.json", idx))
}

// globNames 返回匹配文件的 base name，并按字典序排序。
func globNames(dir string, pattern string) []string {
	matches, _ := filepath.Glob(filepath.Join(dir, pattern))
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, filepath.Base(match))
	}
	sort.Strings(names)
	return names
}

// extractIdx 从类似 request12.json 的文件名里提取数字序号。
func extractIdx(name string) int {
	match := idxPattern.FindStringSubmatch(name)
	if len(match) != 2 {
		return 0
	}
	idx, _ := strconv.Atoi(match[1])
	return idx
}

// latestUpdatedAt 返回目录及其直接子项中最新的修改时间戳。
func latestUpdatedAt(dir string) int64 {
	info, err := os.Stat(dir)
	if err != nil {
		return 0
	}
	latest := info.ModTime().Unix()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return latest
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if ts := info.ModTime().Unix(); ts > latest {
			latest = ts
		}
	}
	return latest
}

// writeJSON 以 UTF-8 JSON 响应写出任意值。
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

// stringsTrimPrefix 在不引入 strings.CutPrefix 依赖的前提下做简单前缀去除。
func stringsTrimPrefix(value string, prefix string) string {
	if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
		return value[len(prefix):]
	}
	return value
}

// splitPath 将请求路径归一化后拆成非空片段。
func splitPath(path string) []string {
	raw := filepath.Clean(path)
	if raw == "." || raw == "/" {
		return nil
	}
	parts := make([]string, 0)
	for _, item := range strings.Split(raw, string(filepath.Separator)) {
		if item != "" && item != "." {
			parts = append(parts, item)
		}
	}
	return parts
}

// mustAssetSubtree 返回内嵌 assets 子目录；初始化失败时直接 panic。
func mustAssetSubtree() fs.FS {
	subtree, err := fs.Sub(assetFS, "assets")
	if err != nil {
		panic(err)
	}
	return subtree
}
