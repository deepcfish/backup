package backup

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	_"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

func Opengui() {
	a := app.NewWithID("Mypack.exe")
	w := a.NewWindow("Mypack")
	w.Resize(fyne.NewSize(800, 600))

	intro := widget.NewLabel("这是一个简单的打包/解包工具。\n请选择需要操作的文件。")

	// 压缩和加密选项
	compressCheck := widget.NewCheck("启用压缩", nil)
	encryptCheck := widget.NewCheck("启用加密", nil)
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("在此输入加密密码")
	encryptCheck.OnChanged = func(checked bool) {
		passwordEntry.Disable()
		if checked {
			passwordEntry.Enable()
		}
	}
	passwordEntry.Disable()

	// 过滤选项
	// 路径过滤
	includePathsEntry := widget.NewEntry()
	includePathsEntry.SetPlaceHolder("包含路径模式，多个用逗号分隔，如: *.txt,subdir/**")
	excludePathsEntry := widget.NewEntry()
	excludePathsEntry.SetPlaceHolder("排除路径模式，多个用逗号分隔，如: *.tmp,*.log")
	
	// 类型过滤
	fileTypeCheck := widget.NewCheck("普通文件", nil)
	dirTypeCheck := widget.NewCheck("目录", nil)
	symlinkTypeCheck := widget.NewCheck("符号链接", nil)
	hardlinkTypeCheck := widget.NewCheck("硬链接", nil)
	
	// 名字过滤
	namePatternsEntry := widget.NewEntry()
	namePatternsEntry.SetPlaceHolder("文件名模式，多个用逗号分隔，如: *.log,test*")
	
	// 时间过滤
	minTimeEntry := widget.NewEntry()
	minTimeEntry.SetPlaceHolder("最小修改时间，格式: 2006-01-02 15:04:05")
	maxTimeEntry := widget.NewEntry()
	maxTimeEntry.SetPlaceHolder("最大修改时间，格式: 2006-01-02 15:04:05")
	
	// 尺寸过滤
	minSizeEntry := widget.NewEntry()
	minSizeEntry.SetPlaceHolder("最小文件大小，如: 1K, 1M, 1G")
	maxSizeEntry := widget.NewEntry()
	maxSizeEntry.SetPlaceHolder("最大文件大小，如: 100M, 1G")
	
	// 创建可折叠的过滤选项容器
	filterForm := container.NewVBox(
		widget.NewLabel("过滤选项（可选）:"),
		widget.NewSeparator(),
		widget.NewLabel("路径过滤:"),
		widget.NewLabel("包含:"),
		includePathsEntry,
		widget.NewLabel("排除:"),
		excludePathsEntry,
		widget.NewSeparator(),
		widget.NewLabel("类型过滤:"),
		container.NewHBox(fileTypeCheck, dirTypeCheck, symlinkTypeCheck, hardlinkTypeCheck),
		widget.NewSeparator(),
		widget.NewLabel("名字过滤:"),
		namePatternsEntry,
		widget.NewSeparator(),
		widget.NewLabel("时间过滤:"),
		widget.NewLabel("最小时间:"),
		minTimeEntry,
		widget.NewLabel("最大时间:"),
		maxTimeEntry,
		widget.NewSeparator(),
		widget.NewLabel("尺寸过滤:"),
		widget.NewLabel("最小大小:"),
		minSizeEntry,
		widget.NewLabel("最大大小:"),
		maxSizeEntry,
	)
	
	filterAccordion := widget.NewAccordion(
		widget.NewAccordionItem("过滤选项", filterForm),
	)

	// 打包按钮
	packBtn := widget.NewButton("打包", func() {
		opt := PackOptions{
			Compress: compressCheck.Checked,
			Encrypt:  encryptCheck.Checked,
			Password: passwordEntry.Text,
		}
		filter := buildFilterFromGUI(
			includePathsEntry.Text,
			excludePathsEntry.Text,
			fileTypeCheck.Checked,
			dirTypeCheck.Checked,
			symlinkTypeCheck.Checked,
			hardlinkTypeCheck.Checked,
			namePatternsEntry.Text,
			minTimeEntry.Text,
			maxTimeEntry.Text,
			minSizeEntry.Text,
			maxSizeEntry.Text,
		)
		PackClicked(w, opt, filter)
	})

	// 解包按钮
	unpackBtn := widget.NewButton("解包", func() {
		opt := PackOptions{
			Password: passwordEntry.Text,
		}
		UnpackClicked(w, opt)
	})

	w.SetContent(container.NewVBox(
		intro,
		widget.NewSeparator(),
		widget.NewLabel("压缩和加密选项:"),
		compressCheck,
		encryptCheck,
		passwordEntry,
		widget.NewSeparator(),
		filterAccordion,
		widget.NewSeparator(),
		packBtn,
		unpackBtn,
	))

	w.ShowAndRun()
}

func PackClicked(w fyne.Window, options PackOptions, filter *Filter) {
	dialog.ShowFolderOpen(func(rootURI fyne.ListableURI, err error) {
		if err != nil || rootURI == nil {
			return
		}
		root := rootURI.Path() //获取路径
		dialog.ShowFileSave(func(save fyne.URIWriteCloser, err error) {
			if err != nil || save == nil {
				return
			}
			archivePath := save.URI().Path()
			save.Close() // 关闭 Fyne 创建的文件句柄, 会创建空文件,待完善
			if fileInfo, err := os.Stat(archivePath); err == nil && fileInfo.Size() == 0 { // 如果 Fyne 创建了空文件，删除它
				os.Remove(archivePath)
			}
			// 验证加密选项
			if options.Encrypt && options.Password == "" {
				dialog.ShowError(fmt.Errorf("启用加密时必须提供密码"), w)
				return
			}
			err = PackWithOptions(root, archivePath + ".tar", filter, options)
			if err != nil {
				dialog.ShowError(err, w)
			} else {
				dialog.ShowInformation("成功", "打包完成", w)
			}
		}, w)
	}, w)
}

func UnpackClicked(w fyne.Window, options PackOptions) {
	dialog.ShowFileOpen(func(archiveURI fyne.URIReadCloser, err error) {
		if err != nil || archiveURI == nil {
			return
		}
		archivePath := archiveURI.URI().Path()
		dialog.ShowFolderOpen(func(destURI fyne.ListableURI, err error) {
			if err != nil || destURI == nil {
				return
			}
			targetPath := destURI.Path()
			if err := UnpackWithOptions(archivePath, targetPath, options); err != nil {
				dialog.ShowError(err, w)
				return
			}
			dialog.ShowInformation("成功", "解包完成", w)
		}, w)
	}, w)
}

// buildFilterFromGUI 从 GUI 输入构建过滤条件
func buildFilterFromGUI(
	includePaths, excludePaths string,
	fileType, dirType, symlinkType, hardlinkType bool,
	namePatterns, minTime, maxTime, minSize, maxSize string,
) *Filter {
	filter := &Filter{}
	
	// 路径过滤
	if includePaths != "" {
		paths := strings.Split(includePaths, ",")
		for _, p := range paths {
			p = strings.TrimSpace(p)
			if p != "" {
				filter.PathPatterns = append(filter.PathPatterns, p)
			}
		}
	}
	if excludePaths != "" {
		paths := strings.Split(excludePaths, ",")
		for _, p := range paths {
			p = strings.TrimSpace(p)
			if p != "" {
				filter.ExcludePaths = append(filter.ExcludePaths, p)
			}
		}
	}
	
	// 类型过滤
	if fileType {
		filter.IncludeTypes = append(filter.IncludeTypes, TypeFile)
	}
	if dirType {
		filter.IncludeTypes = append(filter.IncludeTypes, TypeDir)
	}
	if symlinkType {
		filter.IncludeTypes = append(filter.IncludeTypes, TypeSymlink)
	}
	if hardlinkType {
		filter.IncludeTypes = append(filter.IncludeTypes, TypeHardlink)
	}
	
	// 名字过滤
	if namePatterns != "" {
		patterns := strings.Split(namePatterns, ",")
		for _, p := range patterns {
			p = strings.TrimSpace(p)
			if p != "" {
				filter.NamePatterns = append(filter.NamePatterns, p)
			}
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
	timeStr = strings.TrimSpace(timeStr)
	if timeStr == "" {
		return nil
	}
	
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
