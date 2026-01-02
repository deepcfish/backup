package backup

// FileType 表示文件类型
type FileType int

const (
	TypeFile FileType = iota // 普通文件
	TypeDir                  // 目录
	TypeSymlink              // 符号链接（软链接）
	TypeHardlink             // 硬链接
	TypeFifo                 // 命名管道（FIFO）
	TypeCharDevice           // 字符设备
	TypeBlockDevice          // 块设备
	TypeSocket               // Unix 套接字
)

// FileEntry 表示一个文件/目录的元信息
type FileEntry struct {
	RelPath    string   // 相对于扫描根目录的相对路径，例如 "sub/a.txt"
	Type       FileType // 文件类型
	Mode       uint32   // 权限（从 os.FileMode 转换而来）
	Size       int64    // 文件大小（目录、链接、设备文件为 0）
	ModTime    int64    // 修改时间（Unix 时间戳，秒）
	AccessTime int64    // 访问时间（Unix 时间戳，秒）
	ChangeTime int64    // 状态改变时间（Unix 时间戳，秒）
	UID        int      // 用户ID（属主）
	GID        int      // 组ID（属组）
	LinkTarget string   // 若为符号链接，记录链接目标
	LinkName   string   // 若为硬链接，记录链接到的文件路径（相对于根目录）
	DevMajor   int64    // 设备主编号（设备文件）
	DevMinor   int64    // 设备次编号（设备文件）
	Compress   bool     // 压缩标记
	Encrypt    bool     // 加密标记
}

type PackOptions struct {
    Compress bool      // 是否压缩
    Encrypt  bool	   // 是否加密
    Password string    //密码串
}

