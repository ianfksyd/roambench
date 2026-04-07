package web

import "embed"

//go:embed index.html favicon.svg css js vendor
var StaticFiles embed.FS
