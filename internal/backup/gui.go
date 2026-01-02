package backup

import (
	"fmt"
	"os"
	
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
	//hello := widget.NewLabel("Hello Fyne!")

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

	// 打包按钮
	packBtn := widget.NewButton("打包", func() {
		opt := PackOptions{
			Compress: compressCheck.Checked,
			Encrypt:  encryptCheck.Checked,
			Password: passwordEntry.Text,
		}
		PackClicked(w, opt)
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
		compressCheck,
		encryptCheck,
		passwordEntry,
		widget.NewSeparator(),
		packBtn,
		unpackBtn,
	))

	w.ShowAndRun()
}

func PackClicked(w fyne.Window, options PackOptions) {
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
			err = PackWithOptions(root, archivePath, nil, options)
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
