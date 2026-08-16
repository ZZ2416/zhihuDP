// Package web 前端资源（go:embed 内嵌，不依赖运行目录）
package web

import "embed"

//go:embed index.html css js
var FS embed.FS
