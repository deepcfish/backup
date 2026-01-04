package backup

import (
	"compress/flate"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Unpack 从归档文件解包到指定目录
// archivePath: 归档文件路径（自定义格式，必须是文件，不能是目录）
// restoreRoot: 解包的目标目录
// 返回: 可能的错误
func Unpack(archivePath string, restoreRoot string) error {
	return UnpackWithOptions(archivePath, restoreRoot, PackOptions{})
}

// UnpackWithOptions 从归档文件解包到指定目录（支持解压缩和解密）
// archivePath: 归档文件路径（自定义格式，必须是文件，不能是目录）
// restoreRoot: 解包的目标目录
// options: 解包选项（密码等）
// 返回: 可能的错误
func UnpackWithOptions(archivePath string, restoreRoot string, options PackOptions) error {
	// 验证归档文件路径：必须是文件，不能是目录
	fileInfo, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("归档文件不存在或无法访问: %v", err)
	}
	if fileInfo.IsDir() {
		return fmt.Errorf("归档路径是目录而不是文件: %s", archivePath)
	}
	
	// 打开归档文件
	inFile, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("打开归档文件失败: %v", err)
	}
	defer inFile.Close()
	
	// 读取并验证文件头，获取标志位
	compress, encrypt, err := readHeaderWithFlags(inFile)
	if err != nil {
		return fmt.Errorf("读取文件头失败: %v", err)
	}
	
	// 创建读取链：文件 -> 解密 -> 解压缩 -> 实际读取
	var finalReader io.Reader = inFile
	
	// 如果启用加密，添加解密层
	if encrypt {
		if options.Password == "" {
			return fmt.Errorf("归档文件已加密，需要提供密码")
		}
		// 从密码生成密钥
		key := sha256.Sum256([]byte(options.Password))
		block, err := aes.NewCipher(key[:])
		if err != nil {
			return fmt.Errorf("创建解密器失败: %v", err)
		}
		aesGCM, err := cipher.NewGCM(block)
		if err != nil {
			return fmt.Errorf("创建GCM失败: %v", err)
		}
		// 读取 nonce
		nonceSize := aesGCM.NonceSize()
		nonce := make([]byte, nonceSize)
		if _, err := io.ReadFull(inFile, nonce); err != nil {
			return fmt.Errorf("读取 nonce 失败: %v", err)
		}
		// 创建解密读取器
		finalReader = &decryptReader{
			reader: inFile,
			gcm:    aesGCM,
			nonce:  nonce,
		}
	}
	
	// 如果启用压缩，添加解压缩层
	if compress {
		flateReader := flate.NewReader(finalReader)
		defer flateReader.Close()
		finalReader = flateReader
	}
	
	// 确保目标目录存在
	if err := os.MkdirAll(restoreRoot, 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %v", err)
	}
	
	// 标准化目标路径
	absRestoreRoot, err := filepath.Abs(restoreRoot)
	if err != nil {
		return fmt.Errorf("获取目标绝对路径失败: %v", err)
	}
	
	// 用于硬链接处理的映射（路径 -> 实际文件路径）
	hardlinkMap := make(map[string]string)
	
	// 循环读取条目
	for {
		entryType, err := readEntryType(finalReader)
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("读取条目类型失败: 文件意外结束，可能文件不完整或已损坏")
			}
			return fmt.Errorf("读取条目类型失败: %v", err)
		}
		
		// 检查结束标记
		if entryType == entryTypeEnd {
			break
		}
		
		// 读取条目
		entry, err := readEntry(finalReader, entryType)
		if err != nil {
			return fmt.Errorf("读取条目失败: %v", err)
		}
		
		// 处理路径
		if entry.RelPath == "." {
			continue // 跳过根目录
		}
		
		targetPath := filepath.Join(absRestoreRoot, entry.RelPath)
		
		// 路径安全检查：防止路径逃逸攻击
		relPath, err := filepath.Rel(absRestoreRoot, targetPath)
		if err != nil {
			return fmt.Errorf("路径安全检查失败 (%s): %v", entry.RelPath, err)
		}
		if strings.HasPrefix(relPath, "..") {
			return fmt.Errorf("检测到非法路径逃逸: %s", entry.RelPath)
		}
		
		// 根据文件类型处理
		switch entryType {
		case entryTypeFile:
			if err := restoreFile(finalReader, targetPath, entry); err != nil {
				return err
			}
			
		case entryTypeDir:
			if err := restoreDir(targetPath, entry); err != nil {
				return err
			}
			
		case entryTypeSymlink:
			if err := restoreSymlink(targetPath, entry); err != nil {
				return err
			}
			
		case entryTypeHardlink:
			if err := restoreHardlink(targetPath, entry, absRestoreRoot, hardlinkMap); err != nil {
				return err
			}
			
		case entryTypeFifo:
			if err := restoreFifo(targetPath, entry); err != nil {
				return err
			}
			
		case entryTypeCharDev:
			if err := restoreCharDev(targetPath, entry); err != nil {
				return err
			}
			
		case entryTypeBlockDev:
			if err := restoreBlockDev(targetPath, entry); err != nil {
				return err
			}
			
		default:
			return fmt.Errorf("未知的条目类型: %d", entryType)
		}
	}
	
	return nil
}

// readHeaderWithFlags 读取并验证文件头，返回压缩和加密标志
func readHeaderWithFlags(r io.Reader) (compress, encrypt bool, err error) {
	// 读取魔数
	magic := make([]byte, 4)
	if _, err := io.ReadFull(r, magic); err != nil {
		return false, false, err
	}
	if string(magic) != magicNumber {
		return false, false, fmt.Errorf("无效的归档文件格式，魔数不匹配")
	}
	
	// 读取版本号
	var version uint32
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return false, false, err
	}
	if version < 1 || version > formatVersion {
		return false, false, fmt.Errorf("不支持的归档文件版本: %d", version)
	}
	
	// 读取标志位（版本2+）
	if version >= 2 {
		var flags byte
		if err := binary.Read(r, binary.LittleEndian, &flags); err != nil {
			return false, false, err
		}
		compress = (flags & flagCompress) != 0
		encrypt = (flags & flagEncrypt) != 0
		
		// 跳过保留字段（7字节）
		reserved := make([]byte, 7)
		if _, err := io.ReadFull(r, reserved); err != nil {
			return false, false, err
		}
	} else {
		// 版本1：跳过保留字段（8字节）
		reserved := make([]byte, 8)
		if _, err := io.ReadFull(r, reserved); err != nil {
			return false, false, err
		}
	}
	
	return compress, encrypt, nil
}

// decryptReader 实现解密读取
type decryptReader struct {
	reader   io.Reader
	gcm      cipher.AEAD
	nonce    []byte
	buffer   []byte
	position int
}

func (dr *decryptReader) Read(p []byte) (n int, err error) {
	// 如果缓冲区有数据，先读取缓冲区
	if dr.position < len(dr.buffer) {
		n = copy(p, dr.buffer[dr.position:])
		dr.position += n
		if dr.position >= len(dr.buffer) {
			dr.buffer = nil
			dr.position = 0
		}
		return n, nil
	}
	
	// 读取加密块
	// 每个块的结构：nonce(12字节) + 密文(包含tag，16字节)
	// encryptWriter 先写入 nonce，然后写入密文
	nonceSize := dr.gcm.NonceSize()
	
	// 先读取 nonce
	blockNonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(dr.reader, blockNonce); err != nil {
		if err == io.EOF {
			return 0, io.EOF
		}
		return 0, fmt.Errorf("读取 nonce 失败: %v", err)
	}
	
	// 读取密文（包括 tag）
	// 由于 GCM 的密文长度 = 明文长度 + tag长度(16字节)
	// 我们尝试读取一个合理大小的块
	blockSize := 64 * 1024 // 64KB 明文
	ciphertextSize := blockSize + dr.gcm.Overhead()
	ciphertext := make([]byte, ciphertextSize)
	
	// 使用 ReadFull 读取完整块，但如果遇到 EOF，说明是最后一个块（可能不完整）
	read, err := io.ReadFull(dr.reader, ciphertext)
	isUnexpectedEOF := false
	if err != nil {
		if err == io.EOF {
			// 文件结束，没有更多数据
			return 0, io.EOF
		}
		if err == io.ErrUnexpectedEOF {
			// 最后一个不完整的块，使用实际读取的数据
			ciphertext = ciphertext[:read]
			isUnexpectedEOF = true
		} else {
			return 0, fmt.Errorf("读取密文失败: %v", err)
		}
	}
	
	// 如果读取的数据太少（少于 tag 大小），说明数据不完整
	if len(ciphertext) < dr.gcm.Overhead() {
		if isUnexpectedEOF {
			return 0, io.EOF
		}
		return 0, fmt.Errorf("密文数据不完整: 只读取了 %d 字节", len(ciphertext))
	}
	
	// 解密
	plaintext, err := dr.gcm.Open(nil, blockNonce, ciphertext, nil)
	if err != nil {
		return 0, fmt.Errorf("解密失败（可能是密码错误）: %v", err)
	}
	
	// 将解密后的数据复制到输出
	n = copy(p, plaintext)
	
	// 如果有剩余数据，保存到缓冲区
	if n < len(plaintext) {
		dr.buffer = plaintext[n:]
		dr.position = 0
	}
	
	return n, nil
}

// readEntryType 读取条目类型
func readEntryType(r io.Reader) (byte, error) {
	var entryType byte
	err := binary.Read(r, binary.LittleEndian, &entryType)
	return entryType, err
}

// entryData 表示从归档文件读取的条目数据
type entryData struct {
	RelPath    string
	Type       FileType
	Mode       uint32
	ModTime    int64
	AccessTime int64
	ChangeTime int64
	UID        int32
	GID        int32
	Size       int64
	LinkTarget string
	LinkName   string
	DevMajor   int64
	DevMinor   int64
}

// readEntry 读取一个条目
func readEntry(r io.Reader, entryType byte) (*entryData, error) {
	entry := &entryData{}
	
	// 读取路径
	var pathLen uint32
	if err := binary.Read(r, binary.LittleEndian, &pathLen); err != nil {
		return nil, err
	}
	pathBytes := make([]byte, pathLen)
	if _, err := io.ReadFull(r, pathBytes); err != nil {
		return nil, err
	}
	entry.RelPath = string(pathBytes)
	
	// 读取元数据
	if err := binary.Read(r, binary.LittleEndian, &entry.Mode); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &entry.ModTime); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &entry.AccessTime); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &entry.ChangeTime); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &entry.UID); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &entry.GID); err != nil {
		return nil, err
	}
	
	// 根据条目类型读取特定数据
	switch entryType {
	case entryTypeFile:
		if err := binary.Read(r, binary.LittleEndian, &entry.Size); err != nil {
			return nil, err
		}
		
	case entryTypeSymlink:
		var linkLen uint32
		if err := binary.Read(r, binary.LittleEndian, &linkLen); err != nil {
			return nil, err
		}
		linkBytes := make([]byte, linkLen)
		if _, err := io.ReadFull(r, linkBytes); err != nil {
			return nil, err
		}
		entry.LinkTarget = string(linkBytes)
		
	case entryTypeHardlink:
		var linkLen uint32
		if err := binary.Read(r, binary.LittleEndian, &linkLen); err != nil {
			return nil, err
		}
		linkBytes := make([]byte, linkLen)
		if _, err := io.ReadFull(r, linkBytes); err != nil {
			return nil, err
		}
		entry.LinkName = string(linkBytes)
		
	case entryTypeCharDev, entryTypeBlockDev:
		if err := binary.Read(r, binary.LittleEndian, &entry.DevMajor); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.LittleEndian, &entry.DevMinor); err != nil {
			return nil, err
		}
	}
	
	return entry, nil
}

// restoreFile 恢复普通文件
func restoreFile(r io.Reader, targetPath string, entry *entryData) error {
	// 创建父目录
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("创建父目录失败 (%s): %v", entry.RelPath, err)
	}
	
	// 创建文件
	outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(entry.Mode))
	if err != nil {
		return fmt.Errorf("创建文件失败 (%s): %v", entry.RelPath, err)
	}
	
	// 读取并写入文件内容
	if entry.Size > 0 {
		if _, err := io.CopyN(outFile, r, entry.Size); err != nil {
			outFile.Close()
			return fmt.Errorf("写入文件内容失败 (%s): %v", entry.RelPath, err)
		}
	}
	
	outFile.Close()
	
	// 恢复属主和时间戳
	restoreOwnership(targetPath, int(entry.UID), int(entry.GID))
	restoreTimes(targetPath, entry)
	
	return nil
}

// restoreDir 恢复目录
func restoreDir(targetPath string, entry *entryData) error {
	if err := os.MkdirAll(targetPath, os.FileMode(entry.Mode)); err != nil {
		return fmt.Errorf("创建目录失败 (%s): %v", entry.RelPath, err)
	}
	restoreOwnership(targetPath, int(entry.UID), int(entry.GID))
	restoreTimes(targetPath, entry)
	return nil
}

// restoreSymlink 恢复符号链接
func restoreSymlink(targetPath string, entry *entryData) error {
	// 创建父目录
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("创建父目录失败 (%s): %v", entry.RelPath, err)
	}
	
	// 如果目标路径已存在，先删除
	if _, err := os.Lstat(targetPath); err == nil {
		os.Remove(targetPath)
	}
	
	// 创建符号链接
	if err := os.Symlink(entry.LinkTarget, targetPath); err != nil {
		return fmt.Errorf("创建符号链接失败 (%s -> %s): %v", entry.RelPath, entry.LinkTarget, err)
	}
	
	restoreOwnership(targetPath, int(entry.UID), int(entry.GID))
	return nil
}

// restoreHardlink 恢复硬链接
func restoreHardlink(targetPath string, entry *entryData, absRestoreRoot string, hardlinkMap map[string]string) error {
	// 创建父目录
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("创建父目录失败 (%s): %v", entry.RelPath, err)
	}
	
	// 构造链接目标路径
	linkTarget := filepath.Join(absRestoreRoot, entry.LinkName)
	
	// 检查目标文件是否存在
	if _, err := os.Stat(linkTarget); err != nil {
		// 如果目标文件还不存在，跳过（后续会被处理）
		hardlinkMap[entry.RelPath] = entry.LinkName
		return nil
	}
	
	// 如果目标路径已存在，先删除
	if _, err := os.Lstat(targetPath); err == nil {
		os.Remove(targetPath)
	}
	
	// 创建硬链接
	if err := os.Link(linkTarget, targetPath); err != nil {
		// 硬链接创建失败可能是跨文件系统，降级为复制文件
		if err := copyFile(linkTarget, targetPath); err != nil {
			return fmt.Errorf("创建硬链接失败 (%s -> %s): %v", entry.RelPath, entry.LinkName, err)
		}
	}
	
	restoreOwnership(targetPath, int(entry.UID), int(entry.GID))
	return nil
}

// restoreFifo 恢复命名管道
func restoreFifo(targetPath string, entry *entryData) error {
	// 创建父目录
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("创建父目录失败 (%s): %v", entry.RelPath, err)
	}
	
	// 如果目标路径已存在，先删除
	if _, err := os.Lstat(targetPath); err == nil {
		os.Remove(targetPath)
	}
	
	// 创建命名管道
	if err := syscall.Mkfifo(targetPath, uint32(entry.Mode)); err != nil {
		return fmt.Errorf("创建命名管道失败 (%s): %v", entry.RelPath, err)
	}
	
	restoreOwnership(targetPath, int(entry.UID), int(entry.GID))
	return nil
}

// restoreCharDev 恢复字符设备
func restoreCharDev(targetPath string, entry *entryData) error {
	// 创建父目录
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("创建父目录失败 (%s): %v", entry.RelPath, err)
	}
	
	// 如果目标路径已存在，先删除
	if _, err := os.Lstat(targetPath); err == nil {
		os.Remove(targetPath)
	}
	
	// 创建字符设备
	if err := syscall.Mknod(targetPath, syscall.S_IFCHR|uint32(entry.Mode), int(mkdev(entry.DevMajor, entry.DevMinor))); err != nil {
		return fmt.Errorf("创建字符设备失败 (%s): %v", entry.RelPath, err)
	}
	
	restoreOwnership(targetPath, int(entry.UID), int(entry.GID))
	return nil
}

// restoreBlockDev 恢复块设备
func restoreBlockDev(targetPath string, entry *entryData) error {
	// 创建父目录
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("创建父目录失败 (%s): %v", entry.RelPath, err)
	}
	
	// 如果目标路径已存在，先删除
	if _, err := os.Lstat(targetPath); err == nil {
		os.Remove(targetPath)
	}
	
	// 创建块设备
	if err := syscall.Mknod(targetPath, syscall.S_IFBLK|uint32(entry.Mode), int(mkdev(entry.DevMajor, entry.DevMinor))); err != nil {
		return fmt.Errorf("创建块设备失败 (%s): %v", entry.RelPath, err)
	}
	
	restoreOwnership(targetPath, int(entry.UID), int(entry.GID))
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
func restoreTimes(path string, entry *entryData) {
	atime := time.Unix(entry.ModTime, 0)
	if entry.AccessTime > 0 {
		atime = time.Unix(entry.AccessTime, 0)
	}
	mtime := time.Unix(entry.ModTime, 0)
	// 尝试恢复时间戳，失败不影响主要功能
	_ = os.Chtimes(path, atime, mtime)
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

