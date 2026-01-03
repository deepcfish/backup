package main

import (
	"runtime"

	"backup/internal/backup"
)

func main() {
	runtime.LockOSThread() 	// 锁定OS线程以确保GUI正常工作

	backup.Opengui() 	// 打开GUI窗口
}

