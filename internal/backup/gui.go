package backup
import (
	_"fmt"
	"log"
	"fyne.io/fyne/v2/app"
    "fyne.io/fyne/v2/container"
    "fyne.io/fyne/v2/widget"
)

func opengui() {
    a := app.New()
    w := a.NewWindow("Hello Fyne")

    label := widget.NewLabel("Hello, 小佩可茜忒")
    w.SetContent(container.NewCenter(label))

    w.Resize(fyne.NewSize(400, 300))
    w.ShowAndRun()
}
