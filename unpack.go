package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Unpack 从归档文件解包到指定目录
// archivePath: 归档文件路径（.tar.gz 格式）
// restoreRoot: 解包的目标目录
// 返回: 可能的错误
func Unpack(archivePath string, restoreRoot string) error {
	// 打开归档文件
	inFile, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("打开归档文件失败: %v", err)
	}
	defer inFile.Close()
	
	// 创建 gzip reader
	gr, err := gzip.NewReader(inFile)
	if err != nil {
		return fmt.Errorf("创建 gzip reader 失败: %v", err)
	}
	defer gr.Close()
	
	// 创建 tar reader
	tr := tar.NewReader(gr)
	
	// 确保目标目录存在
	if err := os.MkdirAll(restoreRoot, 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %v", err)
	}
	
	// 标准化目标路径
	absRestoreRoot, err := filepath.Abs(restoreRoot)
	if err != nil {
		return fmt.Errorf("获取目标绝对路径失败: %v", err)
	}
	
	// 循环读取 tar 条目
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // 读取完毕
		}
		if err != nil {
			return fmt.Errorf("读取 tar 条目失败: %v", err)
		}
		
		// 构造目标路径
		// 处理根目录的情况
		targetName := hdr.Name
		if targetName == "." {
			continue // 跳过根目录条目
		}
		
		targetPath := filepath.Join(absRestoreRoot, targetName)
		
		// 路径安全检查：防止路径逃逸攻击
		relPath, err := filepath.Rel(absRestoreRoot, targetPath)
		if err != nil {
			return fmt.Errorf("路径安全检查失败 (%s): %v", targetName, err)
		}
		if strings.HasPrefix(relPath, "..") {
			return fmt.Errorf("检测到非法路径逃逸: %s", targetName)
		}
		
		// 根据文件类型处理
		switch hdr.Typeflag {
		case tar.TypeDir:
			// 创建目录
			if err := os.MkdirAll(targetPath, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("创建目录失败 (%s): %v", targetName, err)
			}
			// 恢复属主（如果可能）
			restoreOwnership(targetPath, hdr.Uid, hdr.Gid)
			// 恢复时间戳
			restoreTimes(targetPath, hdr)
			
		case tar.TypeReg:
			// 创建父目录
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("创建父目录失败 (%s): %v", targetName, err)
			}
			
			// 创建文件
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("创建文件失败 (%s): %v", targetName, err)
			}
			
			// 流式拷贝文件内容
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("写入文件内容失败 (%s): %v", targetName, err)
			}
			
			outFile.Close()
			// 恢复属主
			restoreOwnership(targetPath, hdr.Uid, hdr.Gid)
			// 恢复时间戳
			restoreTimes(targetPath, hdr)
			
		case tar.TypeSymlink:
			// 创建父目录
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("创建父目录失败 (%s): %v", targetName, err)
			}
			
			// 如果目标路径已存在，先删除
			if _, err := os.Lstat(targetPath); err == nil {
				os.Remove(targetPath)
			}
			
			// 创建符号链接
			if err := os.Symlink(hdr.Linkname, targetPath); err != nil {
				return fmt.Errorf("创建符号链接失败 (%s -> %s): %v", targetName, hdr.Linkname, err)
			}
			
		case tar.TypeLink:
			// 硬链接
			// 创建父目录
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("创建父目录失败 (%s): %v", targetName, err)
			}
			
			// 构造链接目标路径
			linkTarget := filepath.Join(absRestoreRoot, hdr.Linkname)
			
			// 检查目标文件是否存在
			if _, err := os.Stat(linkTarget); err != nil {
				// 如果目标文件还不存在，先创建为普通文件（后续会被覆盖）
				// 或者跳过，等目标文件创建后再处理
				// 这里我们尝试创建硬链接，如果失败则记录警告
				continue
			}
			
			// 如果目标路径已存在，先删除
			if _, err := os.Lstat(targetPath); err == nil {
				os.Remove(targetPath)
			}
			
			// 创建硬链接
			if err := os.Link(linkTarget, targetPath); err != nil {
				// 硬链接创建失败可能是跨文件系统，降级为复制文件
				if err := copyFile(linkTarget, targetPath); err != nil {
					return fmt.Errorf("创建硬链接失败 (%s -> %s): %v", targetName, hdr.Linkname, err)
				}
			}
			// 恢复属主
			restoreOwnership(targetPath, hdr.Uid, hdr.Gid)
			
		case tar.TypeFifo:
			// 命名管道
			// 创建父目录
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("创建父目录失败 (%s): %v", targetName, err)
			}
			
			// 如果目标路径已存在，先删除
			if _, err := os.Lstat(targetPath); err == nil {
				os.Remove(targetPath)
			}
			
			// 创建命名管道
			if err := syscall.Mkfifo(targetPath, uint32(hdr.Mode)); err != nil {
				return fmt.Errorf("创建命名管道失败 (%s): %v", targetName, err)
			}
			// 恢复属主
			restoreOwnership(targetPath, hdr.Uid, hdr.Gid)
			
		case tar.TypeChar:
			// 字符设备
			// 创建父目录
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("创建父目录失败 (%s): %v", targetName, err)
			}
			
			// 如果目标路径已存在，先删除
			if _, err := os.Lstat(targetPath); err == nil {
				os.Remove(targetPath)
			}
			
			// 创建字符设备
			if err := syscall.Mknod(targetPath, syscall.S_IFCHR|uint32(hdr.Mode), int(mkdev(hdr.Devmajor, hdr.Devminor))); err != nil {
				return fmt.Errorf("创建字符设备失败 (%s): %v", targetName, err)
			}
			// 恢复属主
			restoreOwnership(targetPath, hdr.Uid, hdr.Gid)
			
		case tar.TypeBlock:
			// 块设备
			// 创建父目录
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("创建父目录失败 (%s): %v", targetName, err)
			}
			
			// 如果目标路径已存在，先删除
			if _, err := os.Lstat(targetPath); err == nil {
				os.Remove(targetPath)
			}
			
			// 创建块设备
			if err := syscall.Mknod(targetPath, syscall.S_IFBLK|uint32(hdr.Mode), int(mkdev(hdr.Devmajor, hdr.Devminor))); err != nil {
				return fmt.Errorf("创建块设备失败 (%s): %v", targetName, err)
			}
			// 恢复属主
			restoreOwnership(targetPath, hdr.Uid, hdr.Gid)
			
		default:
			// 忽略不支持的文件类型（如 socket）
			continue
		}
	}
	
	return nil
}

// restoreOwnership 恢复文件属主（需要 root 权限）
func restoreOwnership(path string, uid, gid int) {
	if uid > 0 || gid > 0 {
		// 尝试恢复属主，失败不影响主要功能
		_ = os.Chown(path, uid, gid)
	}
}

// restoreTimes 恢复文件时间戳
func restoreTimes(path string, hdr *tar.Header) {
	atime := hdr.ModTime
	if !hdr.AccessTime.IsZero() {
		atime = hdr.AccessTime
	}
	// 尝试恢复时间戳，失败不影响主要功能
	_ = os.Chtimes(path, atime, hdr.ModTime)
}

// mkdev 构造设备号
func mkdev(major, minor int64) uint64 {
	return uint64((major << 8) | (minor & 0xff) | ((minor & 0xfff00) << 12))
}

// copyFile 复制文件（用于硬链接降级）
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	
	_, err = io.Copy(dstFile, srcFile)
	return err
}

