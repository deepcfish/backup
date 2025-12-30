package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"backup/internal/backup"
)

func main() {

	backup.opengui()

	var (
		packCmd   = flag.NewFlagSet("pack", flag.ExitOnError)
		unpackCmd = flag.NewFlagSet("unpack", flag.ExitOnError)
	)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "pack":
		source := packCmd.String("source", "", "要打包的源目录或文件路径（必需）")
		output := packCmd.String("output", "", "输出的归档文件路径（必需，建议使用 .tar.gz 扩展名）")
		
		// 过滤选项
		includePaths := packCmd.String("include", "", "包含的路径模式（支持通配符，多个用逗号分隔）")
		excludePaths := packCmd.String("exclude", "", "排除的路径模式（支持通配符，多个用逗号分隔）")
		includeTypes := packCmd.String("types", "", "包含的文件类型（file,dir,symlink,hardlink,fifo,char,block，多个用逗号分隔）")
		namePatterns := packCmd.String("names", "", "文件名模式（支持通配符，多个用逗号分隔）")
		minTime := packCmd.String("min-time", "", "最小修改时间（格式：2006-01-02 15:04:05 或 Unix 时间戳）")
		maxTime := packCmd.String("max-time", "", "最大修改时间（格式：2006-01-02 15:04:05 或 Unix 时间戳）")
		minSize := packCmd.String("min-size", "", "最小文件大小（字节，支持 K/M/G 后缀）")
		maxSize := packCmd.String("max-size", "", "最大文件大小（字节，支持 K/M/G 后缀）")
		
		packCmd.Parse(os.Args[2:])
		
		if *source == "" || *output == "" {
			fmt.Fprintf(os.Stderr, "错误: pack 命令需要 -source 和 -output 参数\n")
			packCmd.Usage()
			os.Exit(1)
		}
		
		// 构建过滤条件
		filter := buildFilter(*includePaths, *excludePaths, *includeTypes, *namePatterns, *minTime, *maxTime, *minSize, *maxSize)
		
		fmt.Printf("正在打包: %s -> %s\n", *source, *output)
		if err := backup.Pack(*source, *output, filter); err != nil {
			fmt.Fprintf(os.Stderr, "打包失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("打包成功: %s\n", *output)
		
	case "unpack":
		archive := unpackCmd.String("archive", "", "要解包的归档文件路径（必需）")
		target := unpackCmd.String("target", "", "解包的目标目录（必需）")
		unpackCmd.Parse(os.Args[2:])
		
		if *archive == "" || *target == "" {
			fmt.Fprintf(os.Stderr, "错误: unpack 命令需要 -archive 和 -target 参数\n")
			unpackCmd.Usage()
			os.Exit(1)
		}
		
		fmt.Printf("正在解包: %s -> %s\n", *archive, *target)
		if err := backup.Unpack(*archive, *target); err != nil {
			fmt.Fprintf(os.Stderr, "解包失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("解包成功: %s\n", *target)
		
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `备份工具 - 目录树打包和解包工具

用法:
  %s pack -source <源路径> -output <归档文件> [过滤选项]
    将指定目录或文件打包到归档文件（.tar.gz 格式）

  %s unpack -archive <归档文件> -target <目标目录>
    从归档文件解包到指定目录

过滤选项（pack 命令）:
  -include <模式>     包含的路径模式（支持通配符，多个用逗号分隔）
  -exclude <模式>     排除的路径模式（支持通配符，多个用逗号分隔）
  -types <类型>       包含的文件类型（file,dir,symlink,hardlink,fifo,char,block，多个用逗号分隔）
  -names <模式>       文件名模式（支持通配符，多个用逗号分隔）
  -min-time <时间>    最小修改时间（格式：2006-01-02 15:04:05 或 Unix 时间戳）
  -max-time <时间>    最大修改时间（格式：2006-01-02 15:04:05 或 Unix 时间戳）
  -min-size <大小>    最小文件大小（字节，支持 K/M/G 后缀，如 1M）
  -max-size <大小>    最大文件大小（字节，支持 K/M/G 后缀，如 100M）

示例:
  %s pack -source /home/user/docs -output backup.tar.gz
  %s pack -source /home/user/docs -output backup.tar.gz -include "*.txt,*.doc" -exclude "*.tmp"
  %s pack -source /home/user/docs -output backup.tar.gz -types "file,dir" -min-size 1K -max-size 10M
  %s unpack -archive backup.tar.gz -target /tmp/restore

`, filepath.Base(os.Args[0]), filepath.Base(os.Args[0]), filepath.Base(os.Args[0]), filepath.Base(os.Args[0]), filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))
}

// buildFilter 从命令行参数构建过滤条件
func buildFilter(includePaths, excludePaths, includeTypes, namePatterns, minTime, maxTime, minSize, maxSize string) *backup.Filter {
	filter := &backup.Filter{}
	
	// 路径过滤
	if includePaths != "" {
		filter.PathPatterns = strings.Split(includePaths, ",")
		for i := range filter.PathPatterns {
			filter.PathPatterns[i] = strings.TrimSpace(filter.PathPatterns[i])
		}
	}
	if excludePaths != "" {
		filter.ExcludePaths = strings.Split(excludePaths, ",")
		for i := range filter.ExcludePaths {
			filter.ExcludePaths[i] = strings.TrimSpace(filter.ExcludePaths[i])
		}
	}
	
	// 类型过滤
	if includeTypes != "" {
		typeStrs := strings.Split(includeTypes, ",")
		for _, ts := range typeStrs {
			ts = strings.TrimSpace(strings.ToLower(ts))
			switch ts {
			case "file":
				filter.IncludeTypes = append(filter.IncludeTypes, backup.TypeFile)
			case "dir":
				filter.IncludeTypes = append(filter.IncludeTypes, backup.TypeDir)
			case "symlink":
				filter.IncludeTypes = append(filter.IncludeTypes, backup.TypeSymlink)
			case "hardlink":
				filter.IncludeTypes = append(filter.IncludeTypes, backup.TypeHardlink)
			case "fifo":
				filter.IncludeTypes = append(filter.IncludeTypes, backup.TypeFifo)
			case "char":
				filter.IncludeTypes = append(filter.IncludeTypes, backup.TypeCharDevice)
			case "block":
				filter.IncludeTypes = append(filter.IncludeTypes, backup.TypeBlockDevice)
			}
		}
	}
	
	// 名字过滤
	if namePatterns != "" {
		filter.NamePatterns = strings.Split(namePatterns, ",")
		for i := range filter.NamePatterns {
			filter.NamePatterns[i] = strings.TrimSpace(filter.NamePatterns[i])
		}
	}
	
	// 时间过滤
	if minTime != "" {
		if t := parseTime(minTime); t != nil {
			filter.MinModTime = t
		}
	}
	if maxTime != "" {
		if t := parseTime(maxTime); t != nil {
			filter.MaxModTime = t
		}
	}
	
	// 尺寸过滤
	if minSize != "" {
		if s := parseSize(minSize); s != nil {
			filter.MinSize = s
		}
	}
	if maxSize != "" {
		if s := parseSize(maxSize); s != nil {
			filter.MaxSize = s
		}
	}
	
	// 如果没有任何过滤条件，返回 nil（表示不过滤）
	if len(filter.PathPatterns) == 0 && len(filter.ExcludePaths) == 0 &&
		len(filter.IncludeTypes) == 0 && len(filter.NamePatterns) == 0 &&
		filter.MinModTime == nil && filter.MaxModTime == nil &&
		filter.MinSize == nil && filter.MaxSize == nil {
		return nil
	}
	
	return filter
}

// parseTime 解析时间字符串
func parseTime(timeStr string) *time.Time {
	// 尝试解析为 Unix 时间戳
	if ts, err := strconv.ParseInt(timeStr, 10, 64); err == nil {
		t := time.Unix(ts, 0)
		return &t
	}
	
	// 尝试解析为常见时间格式
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
		time.RFC3339,
		time.RFC3339Nano,
	}
	
	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return &t
		}
	}
	
	return nil
}

// parseSize 解析大小字符串（支持 K/M/G 后缀）
func parseSize(sizeStr string) *int64 {
	sizeStr = strings.TrimSpace(sizeStr)
	if sizeStr == "" {
		return nil
	}
	
	var multiplier int64 = 1
	sizeStr = strings.ToUpper(sizeStr)
	
	if strings.HasSuffix(sizeStr, "K") {
		multiplier = 1024
		sizeStr = sizeStr[:len(sizeStr)-1]
	} else if strings.HasSuffix(sizeStr, "M") {
		multiplier = 1024 * 1024
		sizeStr = sizeStr[:len(sizeStr)-1]
	} else if strings.HasSuffix(sizeStr, "G") {
		multiplier = 1024 * 1024 * 1024
		sizeStr = sizeStr[:len(sizeStr)-1]
	}
	
	if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
		result := size * multiplier
		return &result
	}
	
	return nil
}

