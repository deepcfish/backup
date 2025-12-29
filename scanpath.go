package backup

import (
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// ScanPath 扫描指定路径下的所有文件和目录，返回文件条目列表
// root: 要扫描的根目录或文件路径
// 返回: 文件条目列表和可能的错误
func ScanPath(root string) ([]FileEntry, error) {
	var entries []FileEntry
	// 用于跟踪硬链接：inode -> 第一个文件路径
	hardlinkMap := make(map[uint64]string)
	
	// 标准化输入路径为绝对路径
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	
	// 获取根路径信息
	rootInfo, err := os.Lstat(absRoot)
	if err != nil {
		return nil, err
	}
	
	// 如果根路径是单个文件，直接处理
	if !rootInfo.IsDir() {
		entry := createFileEntry(absRoot, filepath.Base(absRoot), rootInfo)
		return []FileEntry{entry}, nil
	}
	
	// 如果是目录，使用 WalkDir 遍历
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// 如果访问某个文件出错，记录但继续处理其他文件
			return nil
		}
		
		// 计算相对路径
		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			return nil
		}
		
		// 根目录本身用 "." 表示
		if relPath == "." {
			relPath = "."
		}
		
		// 获取文件信息
		info, err := d.Info()
		if err != nil {
			return nil
		}
		
		entry := createFileEntry(path, relPath, info)
		
		// 检查硬链接
		if sysInfo, ok := info.Sys().(*syscall.Stat_t); ok && sysInfo.Nlink > 1 && entry.Type == TypeFile {
			// 这是一个可能有硬链接的文件
			inode := sysInfo.Ino
			if firstPath, exists := hardlinkMap[inode]; exists {
				// 这是一个硬链接，指向第一个文件
				entry.Type = TypeHardlink
				// 计算第一个文件的相对路径
				firstRelPath, _ := filepath.Rel(absRoot, firstPath)
				entry.LinkName = firstRelPath
			} else {
				// 这是第一个文件，记录它的路径
				hardlinkMap[inode] = path
			}
		}
		
		// 目录路径以 / 结尾，方便后续处理
		if entry.Type == TypeDir && entry.RelPath != "." && entry.RelPath[len(entry.RelPath)-1] != '/' {
			entry.RelPath += "/"
		}
		
		entries = append(entries, entry)
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	
	return entries, nil
}

// createFileEntry 从文件信息创建 FileEntry
func createFileEntry(fullPath, relPath string, info os.FileInfo) FileEntry {
	entry := FileEntry{
		RelPath: relPath,
		Mode:    uint32(info.Mode()),
		Size:    info.Size(),
		ModTime: info.ModTime().Unix(),
	}
	
	// 尝试获取扩展的元数据（UID/GID/时间等）
	if sysInfo, ok := info.Sys().(*syscall.Stat_t); ok {
		entry.UID = int(sysInfo.Uid)
		entry.GID = int(sysInfo.Gid)
		entry.AccessTime = sysInfo.Atim.Sec
		entry.ModTime = sysInfo.Mtim.Sec
		entry.ChangeTime = sysInfo.Ctim.Sec
		
		// 设备文件的主次编号
		if info.Mode()&os.ModeDevice != 0 {
			entry.DevMajor = int64(sysInfo.Rdev >> 8)
			entry.DevMinor = int64(sysInfo.Rdev & 0xff)
		}
	}
	
	// 判断文件类型
	mode := info.Mode()
	switch {
	case info.IsDir():
		entry.Type = TypeDir
		
	case mode&os.ModeSymlink != 0:
		entry.Type = TypeSymlink
		linkTarget, err := os.Readlink(fullPath)
		if err == nil {
			entry.LinkTarget = linkTarget
		}
		
	case mode&os.ModeNamedPipe != 0:
		entry.Type = TypeFifo
		
	case mode&os.ModeSocket != 0:
		entry.Type = TypeSocket
		
	case mode&os.ModeCharDevice != 0:
		entry.Type = TypeCharDevice
		
	case mode&os.ModeDevice != 0:
		entry.Type = TypeBlockDevice
		
	default:
		entry.Type = TypeFile
	}
	
	return entry
}
