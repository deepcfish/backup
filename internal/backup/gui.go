package backup
import (
	_"fmt"
	"log"
	"github.com/gotk3/gotk3/gtk"
)

func opengui() {
    gtk.Init(nil)

    win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
    if err != nil {
        log.Fatal(err)
    }
    win.SetTitle("Hello GTK3")
    win.SetDefaultSize(400, 300)

    win.Connect("destroy", func() {
        gtk.MainQuit()
    })

    label, _ := gtk.LabelNew("Hello, 小佩可茜忒")
    win.Add(label)

    win.ShowAll()
    gtk.Main()
}
