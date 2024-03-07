package main

import (
	"fmt"
	"time"
)

// 函数参数传递的时候,值传递,引用传递, go语言中全部都是值传递
func add(a, b int, c float32) (sum int, err error) {
	sum = a + b
	return sum, err
	//return a + b, nil
}

// 省略号
func add1(desc string, items ...int) (sum int, err error) {
	for _, value := range items {
		sum += value
	}
	return sum, err
}

func runForever() {
	for {
		time.Sleep(time.Second)
		fmt.Println("aaa")
	}
}

func main() {
	//	go函数支持普通函数/匿名函数/闭包
	/**
	go中函数是"一等公民"
		1. 函数本身可以当作变量
		2. 匿名函数 闭包
		3. 函数可以满足接口
	*/

	funcVar := add

	funcVar(3, 4, 2.23)

	sum, _ := add(1, 2, 3.1415)
	sum1, _ := add1("sum", 1, 2, 3, 4)
	fmt.Println(sum)
	fmt.Println(sum1)
}
