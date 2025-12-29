package backup

import (
	"path/filepath"
	"strings"
	"time"
)

// Filter 定义文件过滤条件
type Filter struct {
	// 路径过滤：支持通配符模式，例如 "*.txt", "subdir/**"
	PathPatterns []string // 包含的路径模式（白名单）
	ExcludePaths []string // 排除的路径模式（黑名单）
	
	// 类型过滤：指定要包含的文件类型
	IncludeTypes []FileType // 如果为空，则包含所有类型
	
	// 名字过滤：基于文件名（不含路径）
	NamePatterns []string // 文件名模式，例如 "*.log", "test*"
	
	// 时间过滤：基于修改时间
	MinModTime *time.Time // 最小修改时间
	MaxModTime *time.Time // 最大修改时间
	
	// 尺寸过滤：基于文件大小
	MinSize *int64 // 最小文件大小（字节）
	MaxSize *int64 // 最大文件大小（字节）
}

// Match 检查文件条目是否匹配过滤条件
func (f *Filter) Match(entry FileEntry) bool {
	// 路径过滤
	if len(f.PathPatterns) > 0 {
		matched := false
		for _, pattern := range f.PathPatterns {
			if match, _ := filepath.Match(pattern, entry.RelPath); match {
				matched = true
				break
			}
			// 支持目录通配符匹配
			if strings.Contains(pattern, "**") {
				// 简单的递归匹配
				if strings.HasPrefix(entry.RelPath, strings.Replace(pattern, "**", "", 1)) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return false
		}
	}
	
	// 排除路径过滤
	if len(f.ExcludePaths) > 0 {
		for _, pattern := range f.ExcludePaths {
			if match, _ := filepath.Match(pattern, entry.RelPath); match {
				return false
			}
			// 支持目录通配符匹配
			if strings.Contains(pattern, "**") {
				if strings.HasPrefix(entry.RelPath, strings.Replace(pattern, "**", "", 1)) {
					return false
				}
			}
		}
	}
	
	// 类型过滤
	if len(f.IncludeTypes) > 0 {
		typeMatched := false
		for _, t := range f.IncludeTypes {
			if entry.Type == t {
				typeMatched = true
				break
			}
		}
		if !typeMatched {
			return false
		}
	}
	
	// 名字过滤（基于文件名，不含路径）
	if len(f.NamePatterns) > 0 {
		fileName := filepath.Base(entry.RelPath)
		matched := false
		for _, pattern := range f.NamePatterns {
			if match, _ := filepath.Match(pattern, fileName); match {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	
	// 时间过滤
	if f.MinModTime != nil {
		entryTime := time.Unix(entry.ModTime, 0)
		if entryTime.Before(*f.MinModTime) {
			return false
		}
	}
	if f.MaxModTime != nil {
		entryTime := time.Unix(entry.ModTime, 0)
		if entryTime.After(*f.MaxModTime) {
			return false
		}
	}
	
	// 尺寸过滤（只对普通文件有效）
	if entry.Type == TypeFile {
		if f.MinSize != nil && entry.Size < *f.MinSize {
			return false
		}
		if f.MaxSize != nil && entry.Size > *f.MaxSize {
			return false
		}
	}
	
	return true
}

// ApplyFilter 对文件条目列表应用过滤条件
func ApplyFilter(entries []FileEntry, filter *Filter) []FileEntry {
	if filter == nil {
		return entries
	}
	
	var filtered []FileEntry
	for _, entry := range entries {
		if filter.Match(entry) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

