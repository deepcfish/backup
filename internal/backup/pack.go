package backup

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	// 文件格式魔数和版本
	magicNumber = "BKUP"
	formatVersion = uint32(1)
	
	// 条目类型
	entryTypeEnd      = byte(0) // 文件结束标记
	entryTypeFile     = byte(1) // 普通文件
	entryTypeDir      = byte(2) // 目录
	entryTypeSymlink  = byte(3) // 符号链接
	entryTypeHardlink = byte(4) // 硬链接
	entryTypeFifo     = byte(5) // 命名管道
	entryTypeCharDev  = byte(6) // 字符设备
	entryTypeBlockDev = byte(7) // 块设备
)

// Pack 将指定目录树打包到归档文件
// root: 要打包的源目录或文件路径
// archivePath: 输出的归档文件路径（自定义格式）
// filter: 可选的过滤条件，如果为 nil 则打包所有文件
// 返回: 可能的错误
func Pack(root string, archivePath string, filter *Filter) error {
	// 扫描目录树
	entries, err := ScanPath(root)
	if err != nil {
		return fmt.Errorf("扫描路径失败: %v", err)
	}
	
	// 应用过滤条件
	if filter != nil {
		entries = ApplyFilter(entries, filter)
	}
	
	// 创建输出文件
	outFile, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("创建归档文件失败: %v", err)
	}
	defer outFile.Close()
	
	// 写入文件头
	if err := writeHeader(outFile); err != nil {
		return fmt.Errorf("写入文件头失败: %v", err)
	}
	
	// 标准化源路径
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %v", err)
	}
	
	// 遍历所有条目并写入
	for _, entry := range entries {
		if err := writeEntry(outFile, entry, absRoot); err != nil {
			return fmt.Errorf("写入条目失败 (%s): %v", entry.RelPath, err)
		}
	}
	
	// 写入结束标记
	if err := writeEndMarker(outFile); err != nil {
		return fmt.Errorf("写入结束标记失败: %v", err)
	}
	
	return nil
}

// writeHeader 写入文件头
func writeHeader(w io.Writer) error {
	// 写入魔数（4字节）
	if _, err := w.Write([]byte(magicNumber)); err != nil {
		return err
	}
	
	// 写入版本号（4字节，小端）
	if err := binary.Write(w, binary.LittleEndian, formatVersion); err != nil {
		return err
	}
	
	// 写入保留字段（8字节）
	reserved := make([]byte, 8)
	if _, err := w.Write(reserved); err != nil {
		return err
	}
	
	return nil
}

// writeEntry 写入一个文件条目
func writeEntry(w io.Writer, entry FileEntry, absRoot string) error {
	// 根据文件类型确定条目类型
	var entryType byte
	switch entry.Type {
	case TypeFile:
		entryType = entryTypeFile
	case TypeDir:
		entryType = entryTypeDir
	case TypeSymlink:
		entryType = entryTypeSymlink
	case TypeHardlink:
		entryType = entryTypeHardlink
	case TypeFifo:
		entryType = entryTypeFifo
	case TypeCharDevice:
		entryType = entryTypeCharDev
	case TypeBlockDevice:
		entryType = entryTypeBlockDev
	case TypeSocket:
		// Socket 不支持，跳过
		return nil
	default:
		return fmt.Errorf("未知的文件类型: %d", entry.Type)
	}
	
	// 写入条目类型（1字节）
	if err := binary.Write(w, binary.LittleEndian, entryType); err != nil {
		return err
	}
	
	// 写入路径长度和路径
	pathBytes := []byte(entry.RelPath)
	pathLen := uint32(len(pathBytes))
	if err := binary.Write(w, binary.LittleEndian, pathLen); err != nil {
		return err
	}
	if _, err := w.Write(pathBytes); err != nil {
		return err
	}
	
	// 写入元数据
	if err := binary.Write(w, binary.LittleEndian, entry.Mode); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, entry.ModTime); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, entry.AccessTime); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, entry.ChangeTime); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, int32(entry.UID)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, int32(entry.GID)); err != nil {
		return err
	}
	
	// 根据文件类型写入特定数据
	switch entry.Type {
	case TypeFile:
		// 写入文件大小
		if err := binary.Write(w, binary.LittleEndian, entry.Size); err != nil {
			return err
		}
		// 写入文件内容
		if entry.Size > 0 {
			srcPath := filepath.Join(absRoot, entry.RelPath)
			srcFile, err := os.Open(srcPath)
			if err != nil {
				return fmt.Errorf("打开源文件失败: %v", err)
			}
			if _, err := io.CopyN(w, srcFile, entry.Size); err != nil {
				srcFile.Close()
				return fmt.Errorf("写入文件内容失败: %v", err)
			}
			srcFile.Close()
		}
		
	case TypeSymlink:
		// 写入链接目标
		linkBytes := []byte(entry.LinkTarget)
		linkLen := uint32(len(linkBytes))
		if err := binary.Write(w, binary.LittleEndian, linkLen); err != nil {
			return err
		}
		if _, err := w.Write(linkBytes); err != nil {
			return err
		}
		
	case TypeHardlink:
		// 写入链接目标
		linkBytes := []byte(entry.LinkName)
		linkLen := uint32(len(linkBytes))
		if err := binary.Write(w, binary.LittleEndian, linkLen); err != nil {
			return err
		}
		if _, err := w.Write(linkBytes); err != nil {
			return err
		}
		
	case TypeCharDevice, TypeBlockDevice:
		// 写入设备号
		if err := binary.Write(w, binary.LittleEndian, entry.DevMajor); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, entry.DevMinor); err != nil {
			return err
		}
	}
	
	return nil
}

// writeEndMarker 写入结束标记
func writeEndMarker(w io.Writer) error {
	return binary.Write(w, binary.LittleEndian, entryTypeEnd)
}

