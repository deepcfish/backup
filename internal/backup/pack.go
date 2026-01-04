package backup

import (
	"compress/flate"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	// 文件格式魔数和版本
	magicNumber = "BKUP"
	formatVersion = uint32(2) // 版本2：支持压缩和加密
	
	// 文件头标志位
	flagCompress = byte(0x01) // 压缩标志
	flagEncrypt  = byte(0x02) // 加密标志
	
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
	return PackWithOptions(root, archivePath, filter, PackOptions{})
}

// PackWithOptions 将指定目录树打包到归档文件（支持压缩和加密）
// root: 要打包的源目录或文件路径
// archivePath: 输出的归档文件路径（自定义格式）
// filter: 可选的过滤条件，如果为 nil 则打包所有文件
// options: 打包选项（压缩、加密等）
// 返回: 可能的错误
func PackWithOptions(root string, archivePath string, filter *Filter, options PackOptions) error {
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
	
	// 先写入文件头（不加密不压缩，以便解包时能直接读取）
	if err := writeHeaderWithFlags(outFile, options.Compress, options.Encrypt); err != nil {
		return fmt.Errorf("写入文件头失败: %v", err)
	}
	
	// 创建写入链：文件 -> 加密 -> 压缩 -> 实际写入
	var finalWriter io.Writer = outFile
	var encWriter *encryptWriter // 保存加密写入器的引用
	
	// 如果启用加密，添加加密层
	var aesGCM cipher.AEAD
	if options.Encrypt {
		if options.Password == "" {
			return fmt.Errorf("启用加密时必须提供密码")
		}
		// 从密码生成密钥
		key := sha256.Sum256([]byte(options.Password))
		block, err := aes.NewCipher(key[:])
		if err != nil {
			return fmt.Errorf("创建加密器失败: %v", err)
		}
		aesGCM, err = cipher.NewGCM(block)
		if err != nil {
			return fmt.Errorf("创建GCM失败: %v", err)
		}
		// 生成随机 nonce（12字节）
		nonce := make([]byte, aesGCM.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return fmt.Errorf("生成随机数失败: %v", err)
		}
		// 写入 nonce（在文件头之后）
		if _, err := outFile.Write(nonce); err != nil {
			return fmt.Errorf("写入 nonce 失败: %v", err)
		}
		// 创建加密写入器
		encWriter = &encryptWriter{
			writer: outFile,
			gcm:    aesGCM,
			nonce:  nonce,
		}
		finalWriter = encWriter
	}
	
	// 如果启用压缩，添加压缩层
	if options.Compress {
		flateWriter, err := flate.NewWriter(finalWriter, flate.BestCompression)
		if err != nil {
			return fmt.Errorf("创建压缩器失败: %v", err)
		}
		defer flateWriter.Close()
		finalWriter = flateWriter
	}
	
	// 标准化源路径
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %v", err)
	}
	
	// 遍历所有条目并写入
	for _, entry := range entries {
		if err := writeEntry(finalWriter, entry, absRoot); err != nil {
			return fmt.Errorf("写入条目失败 (%s): %v", entry.RelPath, err)
		}
	}
	
	// 写入结束标记
	if err := writeEndMarker(finalWriter); err != nil {
		return fmt.Errorf("写入结束标记失败: %v", err)
	}
	
	// 如果使用了加密写入器，需要关闭它来刷新缓冲区
	if encWriter != nil {
		if err := encWriter.Close(); err != nil {
			return fmt.Errorf("关闭加密写入器失败: %v", err)
		}
	}
	
	return nil
}

// writeHeaderWithFlags 写入文件头（带压缩和加密标志）
func writeHeaderWithFlags(w io.Writer, compress, encrypt bool) error {
	// 写入魔数（4字节）
	if _, err := w.Write([]byte(magicNumber)); err != nil {
		return err
	}
	
	// 写入版本号（4字节，小端）
	if err := binary.Write(w, binary.LittleEndian, formatVersion); err != nil {
		return err
	}
	
	// 写入标志位（1字节）
	var flags byte
	if compress {
		flags |= flagCompress
	}
	if encrypt {
		flags |= flagEncrypt
	}
	if err := binary.Write(w, binary.LittleEndian, flags); err != nil {
		return err
	}
	
	// 写入保留字段（7字节）
	reserved := make([]byte, 7)
	if _, err := w.Write(reserved); err != nil {
		return err
	}
	
	return nil
}

// encryptWriter 实现加密写入
type encryptWriter struct {
	writer io.Writer
	gcm    cipher.AEAD
	nonce  []byte
	buffer []byte
}

func (ew *encryptWriter) Write(p []byte) (n int, err error) {
	// 将数据添加到缓冲区
	ew.buffer = append(ew.buffer, p...)
	n = len(p)
	
	// 当缓冲区足够大时，加密并写入
	// 使用块大小来分批加密（避免内存过大）
	blockSize := 65536 // 64 * 1024, 64KB
	for len(ew.buffer) >= blockSize {
		chunk := ew.buffer[:blockSize]
		ew.buffer = ew.buffer[blockSize:]
		
		// 生成新的 nonce（对于每个块）
		nonce := make([]byte, ew.gcm.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return 0, err
		}
		
		// 加密数据（nonce 不会被包含在密文中，需要单独写入）
		ciphertext := ew.gcm.Seal(nil, nonce, chunk, nil)
		
		// 先写入 nonce，然后写入密文
		if _, err := ew.writer.Write(nonce); err != nil {
			return 0, err
		}
		if _, err := ew.writer.Write(ciphertext); err != nil {
			return 0, err
		}
	}
	
	return n, nil
}

func (ew *encryptWriter) Close() error {
	// 处理剩余的缓冲区数据
	if len(ew.buffer) > 0 {
		nonce := make([]byte, ew.gcm.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return err
		}
		ciphertext := ew.gcm.Seal(nil, nonce, ew.buffer, nil)
		// 先写入 nonce，然后写入密文
		if _, err := ew.writer.Write(nonce); err != nil {
			return err
		}
		if _, err := ew.writer.Write(ciphertext); err != nil {
			return err
		}
		ew.buffer = nil
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

