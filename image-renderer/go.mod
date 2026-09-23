module github.com/hankchen/go-canvas/image-renderer

go 1.25.0

// 核心 module 与后端 module 同仓嵌套,后端经 replace 指向本地核心
// (核心尚未发远端仓库,与 composer path repository 同一处境)
replace github.com/hankchen/go-canvas => ../

require (
	github.com/hankchen/go-canvas v0.0.0-00010101000000-000000000000
	golang.org/x/image v0.45.0
)

require (
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
