package paser

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
)

func main() {
	apkFile := "com.xxx.9.7_32bit.apk"
	app, err := NewAppParser(apkFile, "keytool", false)
	marshal, err := json.Marshal(app)
	if err != nil {
		return
	}
	if app.Icon != nil {
		// 生成png的icon
		pngFile, err := os.Create("./helloworld.png")
		defer func() {
			_ = pngFile.Close()
		}()
		// 将 img 保存为 PNG 格式的图片文件
		err = png.Encode(pngFile, app.Icon)
		if err != nil {
		}
	}
	fmt.Println(string(marshal))
	fmt.Println(err)
}
