package backup
import (
	_"fmt"
	"fyne.io/fyne/v2/app"
    "fyne.io/fyne/v2/container"
    "fyne.io/fyne/v2/widget"
	"fyne.io/fyne/v2/dialog"
)

func opengui() {
    a := app.New()
    w := a.NewWindow("Karin")

    // Label
    label := widget.NewLabel("Hello, 小佩可茜忒")

    // Button 打开文件对话框
    openButton := widget.NewButton("选择文件", func() {
        fd := dialog.NewFileOpen(
            func(reader fyne.URIReadCloser, err error) {
                if err != nil {
                    dialog.ShowError(err, w)
                    return
                }
                if reader == nil {
                    // 用户取消
                    return
                }
                path := reader.URI().Path()
                label.SetText("选择的文件: " + path)

                // TODO: 在这里调用你的打包软件逻辑
                fmt.Println("用户选择了文件:", path)
            }, w)
        fd.SetFilter(storage.NewExtensionFileFilter([]string{".zip", ".txt"})) // 可选文件类型
        fd.Show()
    })

    // 布局：垂直排列 Label + Button
    content := container.NewVBox(label, openButton)
    w.SetContent(container.NewCenter(content))

    w.Resize(fyne.NewSize(800, 600))
    w.ShowAndRun()
}
