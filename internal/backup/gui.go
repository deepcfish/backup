package backup

import (
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
	hello := widget.NewLabel("Hello Fyne!")

	w.SetContent(container.NewVBox(
		intro,
		widget.NewButton("Hi!", func() {
			hello.SetText("Welcome :)")
		}),
		widget.NewButton("打包", func() {
			PackClicked(w)
		}),
		widget.NewButton("解包", func() {
			UnpackClicked(w)
		}),
	))

	w.ShowAndRun()
}

func PackClicked(w fyne.Window) {
	dialog.ShowFolderOpen(func(rootURI fyne.ListableURI, err error) {
		if err != nil || rootURI == nil {
			return
		}
		root := rootURI.Path() //获取路径
		dialog.ShowFileSave(func(save fyne.URIWriteCloser, err error) {
			if err != nil || save == nil {
				return
			}
			defer save.Close()
			err = Pack(root, save.URI().Path(), nil)
			if err != nil {
				dialog.ShowError(err, w)
			} else {
				dialog.ShowInformation("成功", "打包完成", w)
			}
		}, w)
	}, w)
}

func UnpackClicked (w fyne.Window) {
	dialog.ShowFolderOpen(func(rootURI fyne.ListableURI, err error) {
		if err != nil || rootURI == nil {
			return
		}
		root := rootURI.Path() //获取路径
		dialog.ShowFileSave(func(save fyne.URIWriteCloser, err error) {
			if err != nil || save == nil {
				return
			}
			defer save.Close()
			err = Unpack(root, save.URI().Path())
			if err != nil {
				dialog.ShowError(err, w)
			} else {
				dialog.ShowInformation("成功", "解包完成", w)
			}
		}, w)
	}, w)
}
