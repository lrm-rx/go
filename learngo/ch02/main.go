package main

import "fmt"

func main() {
	//var a int8
	//var b int16
	//var c int32
	//var d int64
	//var ua uint8
	//var ub uint16
	//var uc uint32
	//var ud uint64
	//var e int // 动态类型
	//
	//a = int8(b)
	//
	//var f1 float32 // 大约是3.4e38
	//var f2 float64 // 1.8
	//
	//var f1 = 3.14
	//var f2 = 3.14

	var c byte // 主要适用于存放字符
	var c2 rune // 也是字符
	c2 = '明'
	c = 'a' + 1
	//c1 := 97
	fmt.Printf("c=%c",c)
	fmt.Println()
	fmt.Printf("c=%c", c2)
	fmt.Println()

	var name string
	name = "ming"
	fmt.Println(name)

}
