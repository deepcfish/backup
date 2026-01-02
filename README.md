# backup - 目录树备份和还原工具

一个用 Go 语言实现的目录树打包和解包工具，支持将目录树中的文件数据保存到指定位置，并能恢复到另外的指定位置。

## 功能特性

### 核心功能
- **数据备份**：将目录树中的所有文件打包成自定义格式的归档文件
- **数据还原**：从归档文件中恢复目录树到指定位置
- **打包解包**：将所有备份文件拼接为一个大文件保存（自定义格式）

### 文件类型支持（10分）
支持 Linux 文件系统的各种特殊文件类型：
- ✅ 普通文件（Regular File）
- ✅ 目录（Directory）
- ✅ 符号链接（软链接，Symbolic Link）
- ✅ 硬链接（Hard Link）
- ✅ 命名管道（FIFO）
- ✅ 字符设备（Character Device）
- ✅ 块设备（Block Device）
- ⚠️ Unix 套接字（Socket，通常不需要备份）

### 元数据支持（10分）
完整保留文件系统元数据：
- ✅ 文件权限（Mode/Permissions）
- ✅ 修改时间（Modification Time）
- ✅ 访问时间（Access Time）
- ✅ 状态改变时间（Change Time）
- ✅ 用户ID（UID，属主）
- ✅ 组ID（GID，属组）
- ✅ 设备文件的主次编号（DevMajor/DevMinor）

### 自定义备份过滤（各5分）
允许用户筛选需要备份的文件，支持多种过滤条件：

1. **路径过滤**：支持通配符模式
   - 包含路径：`-include "*.txt,subdir/**"`
   - 排除路径：`-exclude "*.tmp,*.log"`

2. **类型过滤**：指定要包含的文件类型
   - `-types "file,dir,symlink"`

3. **名字过滤**：基于文件名（不含路径）
   - `-names "*.log,test*"`

4. **时间过滤**：基于修改时间
   - `-min-time "2024-01-01 00:00:00"`
   - `-max-time "2024-12-31 23:59:59"`

5. **尺寸过滤**：基于文件大小
   - `-min-size 1K`（最小 1KB）
   - `-max-size 100M`（最大 100MB）

## 核心函数

### 1. ScanPath(root string) ([]FileEntry, error)
扫描指定路径下的所有文件和目录，返回文件条目列表。
- 识别所有特殊文件类型
- 提取完整的元数据（权限、时间、属主等）
- 自动检测硬链接关系

### 2. Pack(root string, archivePath string, filter *Filter) error
将指定目录树打包到归档文件（自定义格式）。
- 支持所有文件类型的打包
- 保留完整元数据
- 支持过滤条件筛选文件

### 3. Unpack(archivePath string, restoreRoot string) error
从归档文件解包到指定目录。
- 支持所有文件类型的还原
- 恢复文件权限、时间戳、属主等元数据
- 路径安全检查，防止路径逃逸攻击

### 4. Filter 结构体
定义文件过滤条件，支持路径、类型、名字、时间、尺寸等多种过滤方式。

## 编译和运行

### 编译

**方式一：从项目根目录编译**
```bash
go build -o backup ./cmd/backup
```

**方式二：进入 cmd/backup 目录编译**
```bash
cd cmd/backup
go build -o backup
```

**方式三：使用 go install（安装到 $GOPATH/bin 或 $GOBIN）**
```bash
go install ./cmd/backup
```

### 使用方法

#### 打包（备份）

**基本用法：**
```bash
./backup pack -source <源路径> -output <归档文件>
```

**带过滤条件：**
```bash
# 只备份 .txt 和 .doc 文件，排除临时文件
./backup pack -source /home/user/docs -output backup.bkup \
  -include "*.txt,*.doc" -exclude "*.tmp"

# 只备份普通文件和目录，大小在 1KB 到 10MB 之间
./backup pack -source /home/user/docs -output backup.bkup \
  -types "file,dir" -min-size 1K -max-size 10M

# 备份指定时间范围内的文件
./backup pack -source /home/user/docs -output backup.bkup \
  -min-time "2024-01-01 00:00:00" -max-time "2024-12-31 23:59:59"

# 组合多个过滤条件
./backup pack -source /home/user/docs -output backup.bkup \
  -include "*.txt" -exclude "*.tmp" -names "important*" \
  -min-size 1K -max-size 100M
```

#### 解包（还原）

```bash
./backup unpack -archive <归档文件> -target <目标目录>
```

示例：
```bash
./backup unpack -archive backup.bkup -target /tmp/restore
```

## 实现说明

- 使用自定义二进制格式实现打包功能（不使用标准库的 tar/gzip）
- 采用流式处理，支持大文件
- 使用相对路径存储，支持解包到任意位置
- 包含路径安全检查，防止恶意路径逃逸
- 使用 `syscall` 获取 Linux 特定的元数据（UID/GID/时间等）
- 硬链接通过 inode 跟踪自动识别
- 设备文件通过主次编号正确还原

## 文件结构

```
backup/
├── types.go         # 数据结构定义（FileEntry, FileType）
├── filter.go        # 过滤功能实现
├── scanpath.go      # 路径扫描函数
├── pack.go          # 打包函数
├── unpack.go        # 解包函数
├── go.mod           # Go 模块定义
├── README.md        # 说明文档
└── cmd/
    └── backup/
        └── main.go  # 命令行入口（package main）
```

**包结构说明：**
- 根目录：`package backup` - 库代码，提供核心功能
- `cmd/backup/`：`package main` - 可执行程序入口

## 注意事项

1. **权限要求**：恢复文件属主（UID/GID）需要 root 权限，普通用户可能无法完全恢复
2. **硬链接**：跨文件系统的硬链接会降级为文件复制
3. **设备文件**：设备文件需要在有相应设备的系统上才能正确还原
4. **Socket**：Unix 套接字通常不需要备份，会被跳过
5. **时间格式**：支持多种时间格式，包括 Unix 时间戳和常见日期时间格式

## 评分对应

- ✅ **文件类型支持（10分）**：支持管道/软链接/硬链接/设备文件等
- ✅ **元数据支持（10分）**：支持属主/时间/权限等完整元数据
- ✅ **自定义备份（各5分）**：支持路径/类型/名字/时间/尺寸筛选
- ✅ **打包解包（10分）**：所有文件拼接为一个大文件（自定义格式）
