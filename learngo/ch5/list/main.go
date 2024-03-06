package main

import (
	"container/list"
	"fmt"
)

func main() {
	// 初始化方式一:
	//var myList list.List

	// 初始化方式二:
	myList := list.New()

	myList.PushBack("go")
	myList.PushBack("python")
	myList.PushBack("java")
	myList.PushBack("vue")

	// 头部插入数据
	myList.PushFront("react")

	//myList.InsertBefore("js", i)
	fmt.Println("aa:", myList)

	// 遍历打印值 正序
	for i := myList.Front(); i != nil; i = i.Next() {
		fmt.Println(i.Value)
	}

	// 反向遍历
	//for i := myList.Back(); i != nil; i = i.Prev() {
	//	fmt.Println(i.Value)
	//}

	/**
	集合类型4种:
		1. 数组 - 不同长度的数组类型不一样
		2. 切片 - 动态数组,用起来很方便,而且性能高,尽量使用
		3. map
		4. list - 用得少
	*/
}
