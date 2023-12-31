package paser

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	androidExt = ".apk"
)

type AppInfo struct {
	Name            string      // 应用名称
	BundleId        string      // 包名
	Version         string      // 版本名称
	Build           int         // 版本号
	Icon            image.Image // app icon
	Size            int64       // app size in bytes
	SignatureMd5    string      // app sign
	SignatureSha1   string      // app sign
	SignatureSha256 string      // app sign
	Md5             string      // app md5
	UsesPermission  []string    //权限
	SupportOS64     bool        // 是否支持64位
	SupportOS32     bool        // 是否支持32位
}

type androidManifest struct {
	Package        string `xml:"package,attr"`
	VersionName    string `xml:"versionName,attr"`
	VersionCode    string `xml:"versionCode,attr"`
	UsesPermission []struct {
		Text string `xml:",chardata"`
		Name string `xml:"name,attr"`
	} `xml:"uses-permission"`
	Permission []struct {
		Text            string `xml:",chardata"`
		Name            string `xml:"name,attr"`
		ProtectionLevel string `xml:"protectionLevel,attr"`
	} `xml:"permission"`
}

func NewAppParser(name, keyToolPath string, isIcon bool) (*AppInfo, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	} else if filepath.Ext(stat.Name()) != androidExt {
		return nil, errors.New("unknown platform")
	}

	reader, err := zip.NewReader(file, stat.Size())
	if err != nil {
		return nil, err
	}

	var (
		xmlFile     *zip.File
		supportOS64 bool
		supportOS32 bool
		hasSoFile   bool
	)
	for _, f := range reader.File {
		switch f.Name {
		case "AndroidManifest.xml":
			xmlFile = f
		}
		if strings.HasSuffix(f.Name, ".so") {
			hasSoFile = true
		}
		if strings.HasPrefix(f.Name, "lib/arm64-v8a") {
			supportOS64 = true
		}
		if strings.HasPrefix(f.Name, "lib/armeabi") {
			supportOS32 = true
		}
	}
	info, errParse := parseApkFile(xmlFile)
	if errParse != nil {
		return nil, errParse
	}
	// 当前apk支持的系统位数
	if hasSoFile == false && supportOS64 == false && supportOS32 == false {
		info.SupportOS64 = true
		info.SupportOS32 = true
	} else {
		info.SupportOS64 = supportOS64
		info.SupportOS32 = supportOS32
	}
	apkMd5, _ := getApkMd5(file)
	info.Md5 = apkMd5
	info.SignatureMd5, info.SignatureSha1, info.SignatureSha256 = getSignature(name, keyToolPath)

	icon, label, errExtra := parseApkIconAndLabel(name)
	if errExtra != nil {
		return nil, errExtra
	}
	info.Name = label
	if isIcon {
		info.Icon = icon
	}
	info.Size = stat.Size()

	return info, err
}

// 解析apk文件
func parseApkFile(xmlFile *zip.File) (*AppInfo, error) {
	if xmlFile == nil {
		return nil, errors.New("AndroidManifest.xml not found")
	}

	manifest, err := parseAndroidManifest(xmlFile)
	if err != nil {
		return nil, err
	}

	info := new(AppInfo)
	versionCode, _ := strconv.Atoi(manifest.VersionCode)

	info.BundleId = manifest.Package
	info.Version = manifest.VersionName
	info.Build = versionCode

	for _, permission := range manifest.UsesPermission {
		info.UsesPermission = append(info.UsesPermission, permission.Name)
	}
	return info, nil
}

// 解析AndroidManifest.xml文件
func parseAndroidManifest(xmlFile *zip.File) (*androidManifest, error) {
	rc, err := xmlFile.Open()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rc.Close()
	}()

	buf, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	xmlContent, err := NewXMLFile(bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}

	manifest := new(androidManifest)
	decoder := xml.NewDecoder(xmlContent.Reader())
	if err := decoder.Decode(manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

// 解析apk图标和名称
func parseApkIconAndLabel(name string) (image.Image, string, error) {
	pkg, err := openFile(name)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		_ = pkg.close()
	}()

	icon, _ := pkg.icon(&ResTableConfig{
		Density: 720,
	})

	label, _ := pkg.label(nil)

	return icon, label, nil
}

// 获取apk md5
func getApkMd5(file *os.File) (string, error) {
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%032x", hash.Sum(nil)), nil
}

// 获取apk签名
func getSignature(apkPath, keyToolPath string) (string, string, string) {
	if apkPath == "" || keyToolPath == "" {
		return "", "", ""
	}
	keytoolCmd := exec.Command(keyToolPath, "-printcert", "-jarfile", apkPath)

	// 设置管道连接各个命令
	var (
		output       bytes.Buffer
		resultMD5    string
		resultSHA1   string
		resultSHA256 string
	)
	keytoolCmd.Stdout = &output
	// 运行命令
	if errRun := keytoolCmd.Run(); errRun != nil {
		return "", "", ""
	}

	// 将字符串拆分成多行
	lines := strings.Split(output.String(), "\n")
	// 匹配规则：包含字符串 "MD5:"
	for _, line := range lines {
		if strings.Contains(line, "MD5:") {
			_, resultMD5, _ = strings.Cut(line, "MD5:")
			continue
		}
		if strings.Contains(line, "SHA1:") {
			_, resultSHA1, _ = strings.Cut(line, "SHA1:")
			continue
		}
		if strings.Contains(line, "SHA256:") {
			_, resultSHA256, _ = strings.Cut(line, "SHA256:")
			continue
		}
	}
	// 将匹配结果拼接成一个新的字符串
	resultMD5 = strings.Replace(resultMD5, " ", "", -1)
	resultMD5 = strings.Replace(resultMD5, ":", "", -1)

	resultSHA1 = strings.Replace(resultSHA1, " ", "", -1)
	resultSHA1 = strings.Replace(resultSHA1, ":", "", -1)

	resultSHA256 = strings.Replace(resultSHA256, " ", "", -1)
	resultSHA256 = strings.Replace(resultSHA256, ":", "", -1)

	return strings.ToLower(resultMD5), strings.ToLower(resultSHA1), strings.ToLower(resultSHA256)
}
