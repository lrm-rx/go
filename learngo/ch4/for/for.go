package main

import (
	"fmt"
	"time"
)

func main() {
	// for 循环
	/*
		for init; condition; post {}
	*/
	//for i := 0; i<3; i++ {
	//	fmt.Println(i)
	//}

	//var i int
	//for {
	//	time.Sleep(time.Second)
	//	fmt.Println(i)
	//	i++
	//}

	//var sum int
	//for i := 1; i <= 100; i++ {
	//	sum += i
	//}
	//fmt.Print(sum)
	//
	//for y := 1; y <= 9; y++ {
	//	for x := 1; x <= y; x++ {
	//		fmt.Printf("%d*%d=%d\t", x, y, x*y)
	//	}
	//	fmt.Println()
	//}

	// for 循环还有一种用法, for range, 主要是对 字符串, 数组, 切片, map, channel

	/*
		for key,value := range {}
	*/

	//name := "imooc go体系课"
	//nameRune := []rune(name)
	//for index, value := range name {
	//	//fmt.Println(index, value)
	//	fmt.Printf("%d, %c\r\n", index, value)
	//}

	//for i := 0; i < len(nameRune); i++ {
	//	fmt.Printf(" %c\r\n", nameRune[i])
	//}

	// 字符串   字符串的索引(index)   字符串对应的索引的字符值的拷贝(value)  如果不写index, 那么返回的是索引
	// 数组     数组的索引			  索引对应的值的拷贝					如果不写index, 那么返回的是索引
	// 切片	   切片的索引           索引对应的值的拷贝					如果不写index, 那么返回的是索引
	// map		map的index         value返回的是index对应的值的拷贝       如果不写index, 那么返回的是map的值
	// channel  				 value返回的是channel接受的数据

	round := 0
	for {
		time.Sleep(1 * time.Second)
		round++
		if round == 5 {
			continue
		}
		fmt.Println(round)
		if round > 10 {
			break
		}
	}
}
