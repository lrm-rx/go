package md5

import (
	"crypto/md5"
	"fmt"
	"io"
)

func ToMd5(s string) string {
	m := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", m)
}

func FileToMd5(file io.Reader) string {
	byteData, _ := io.ReadAll(file)
	return ToMd5(string(byteData))
}
