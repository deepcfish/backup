package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Pack 将指定目录树打包到归档文件
// root: 要打包的源目录或文件路径
// archivePath: 输出的归档文件路径（.tar.gz 格式）
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
	
	// 创建 gzip writer
	gw := gzip.NewWriter(outFile)
	defer gw.Close()
	
	// 创建 tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()
	
	// 标准化源路径
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %v", err)
	}
	
	// 遍历所有条目并写入 tar
	for _, entry := range entries {
		// 构造 tar header
		hdr := &tar.Header{
			Name:    entry.RelPath,
			Mode:    int64(entry.Mode),
			ModTime: time.Unix(entry.ModTime, 0),
			Uid:     entry.UID,
			Gid:     entry.GID,
		}
		
		// 设置访问时间和状态改变时间（如果支持）
		if entry.AccessTime > 0 {
			hdr.AccessTime = time.Unix(entry.AccessTime, 0)
		}
		if entry.ChangeTime > 0 {
			hdr.ChangeTime = time.Unix(entry.ChangeTime, 0)
		}
		
		// 根据文件类型设置 header
		switch entry.Type {
		case TypeDir:
			hdr.Typeflag = tar.TypeDir
			hdr.Size = 0
			// 确保目录名以 / 结尾
			if hdr.Name != "." && hdr.Name[len(hdr.Name)-1] != '/' {
				hdr.Name += "/"
			}
			
		case TypeSymlink:
			hdr.Typeflag = tar.TypeSymlink
			hdr.Linkname = entry.LinkTarget
			hdr.Size = 0
			
		case TypeHardlink:
			hdr.Typeflag = tar.TypeLink
			hdr.Linkname = entry.LinkName
			hdr.Size = 0
			
		case TypeFifo:
			hdr.Typeflag = tar.TypeFifo
			hdr.Size = 0
			
		case TypeCharDevice:
			hdr.Typeflag = tar.TypeChar
			hdr.Size = 0
			hdr.Devmajor = entry.DevMajor
			hdr.Devminor = entry.DevMinor
			
		case TypeBlockDevice:
			hdr.Typeflag = tar.TypeBlock
			hdr.Size = 0
			hdr.Devmajor = entry.DevMajor
			hdr.Devminor = entry.DevMinor
			
		case TypeSocket:
			// tar 格式不支持 socket，我们跳过或作为特殊标记
			// 在实际应用中，socket 通常不需要备份
			continue
			
		case TypeFile:
			hdr.Typeflag = tar.TypeReg
			hdr.Size = entry.Size
		}
		
		// 写入 header
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("写入 tar header 失败 (%s): %v", entry.RelPath, err)
		}
		
		// 如果是普通文件或设备文件，写入文件内容（设备文件通常不需要内容）
		if entry.Type == TypeFile {
			srcPath := filepath.Join(absRoot, entry.RelPath)
			srcFile, err := os.Open(srcPath)
			if err != nil {
				return fmt.Errorf("打开源文件失败 (%s): %v", srcPath, err)
			}
			
			// 流式拷贝文件内容
			if _, err := io.Copy(tw, srcFile); err != nil {
				srcFile.Close()
				return fmt.Errorf("写入文件内容失败 (%s): %v", entry.RelPath, err)
			}
			
			srcFile.Close()
		}
	}
	
	return nil
}

