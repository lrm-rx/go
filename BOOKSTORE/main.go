package main

import "fmt"

func main() {
	// 连接数据库
	db, err := NewDB("bookstore.db")
	if err != nil {
		fmt.Printf("连接数据库失败: %v\n", err)
		return
	}
	fmt.Println(db)
}
