module github.com/HankChenCH/go-canvas/image-renderer

go 1.25.0

// 核心 module 与后端 module 同仓嵌套,后端经 replace 指向本地核心
// (核心尚未发远端仓库,与 composer path repository 同一处境)
replace github.com/HankChenCH/go-canvas => ../

require (
	github.com/go-text/typesetting v0.3.5
	github.com/HankChenCH/go-canvas v0.0.0-00010101000000-000000000000
	github.com/yeqown/go-qrcode/v2 v2.3.0
	github.com/yeqown/go-qrcode/writer/standard v1.4.0
	golang.org/x/image v0.45.0
)

require (
	github.com/fogleman/gg v1.3.0 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/yeqown/reedsolomon v1.0.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
